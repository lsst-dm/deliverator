package conntracker

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	gherrors "github.com/pkg/errors"
	"golang.org/x/sys/unix"
)

// ConnTracker records and tracks every conn the transport dials. A conn is
// removed from the active list when it is closed by http.Transport.
type ConnTracker struct {
	dialer           *net.Dialer
	mu               sync.Mutex
	active           map[*TrackedConn]struct{}
	tcpInfoSumClosed *unix.TCPInfo
	closed           uint64        // number of closed connections
	uploadPace       atomic.Uint64 // pace in *bytes* per second for uploads
}

type ConnTrackerConnections struct {
	Active uint64 // number of active connections
	Closed uint64 // number of closed connections
}

func NewConnTracker(d *net.Dialer) *ConnTracker {
	t := &ConnTracker{
		dialer:           d,
		active:           make(map[*TrackedConn]struct{}),
		tcpInfoSumClosed: &unix.TCPInfo{},
	}

	// enable periodic stats logging -- useful for debugging
	// t.startConnStats()

	return t
}

// returns the current upload pacing rate in *bytes per second*.
func (t *ConnTracker) PacingRate() uint64 {
	return t.uploadPace.Load()
}

// returns the current upload pacing rate in *megabits per second*.
func (t *ConnTracker) PacingRateMbits() float64 {
	return float64(t.PacingRate()*8) / (1 << 20)
}

// sets the upload pacing rate in *bytes per second*. A value of 0 means
// no pacing is applied.
func (t *ConnTracker) SetPacingRate(pace uint64) error {
	if pace == t.PacingRate() {
		// no change, no need to do anything
		return nil
	}

	t.uploadPace.Store(pace)

	var e []error

	t.Monkey(func(active map[*TrackedConn]struct{}) {
		for conn := range active {
			if err := conn.setPacingRate(pace); err != nil {
				e = append(e, err)
				continue
			}
		}
	})

	return errors.Join(e...)
}

// Allow tinkering with the tracked active and idle pools. Not for the faint of
// heart.
//
//	// h.conntracker.Monkey(func(active map[net.Conn]struct{}) {
//	// 	updateConn := func(c net.Conn) error {
//	// 		sc, ok := c.(syscall.Conn)
//	// 		if !ok {
//	// 			return errors.New("unable to cast net.Conn to syscall.Conn")
//	// 		}
//	// 		rc, err := sc.SyscallConn()
//	// 		if err != nil {
//	// 			return errors.Wrap(err, "unable to obtain syscall.RawConn from net.Conn")
//	// 		}
//
//	// 		var operr error
//	// 		if err := rc.Control(func(fd uintptr) {
//	// 			operr = syscall.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_MAX_PACING_RATE, int(h.Pace()))
//	// 		}); err != nil {
//	// 			return errors.Wrap(err, "unable to set SO_MAX_PACING_RATE on net.Conn")
//	// 		}
//	// 		if operr != nil {
//	// 			return errors.Wrap(operr, "unable to set SO_MAX_PACING_RATE on net.Conn")
//	// 		}
//
//	// 		return nil
//	// 	}
//
//	// 	for conn := range active {
//	// 		if err := updateConn(conn); err != nil {
//	// 			panic(errors.Wrap(err, "unable to update connection pacing rate"))
//	// 		}
//	// 	}
//	// })
func (t *ConnTracker) Monkey(do func(active map[*TrackedConn]struct{})) {
	t.mu.Lock()
	defer t.mu.Unlock()
	do(t.active)
}

// DialContext satisfies http.Transport.DialContext.
func (t *ConnTracker) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	conn, err := t.dialer.DialContext(ctx, network, addr)
	if err != nil {
		return nil, err
	}

	tc, ok := conn.(TrackableConn)
	if !ok {
		return nil, gherrors.New("unable to cast net.Conn to TrackableConn")
	}

	// Wrap the conn so we can see when the request finishes and
	// the transport calls Close().
	tConn := newTrackedConn(tc, t)

	t.mu.Lock()
	t.active[tConn] = struct{}{}
	t.mu.Unlock()

	if t.PacingRate() > 0 {
		return tConn, tConn.setPacingRate(t.PacingRate())
	}

	return tConn, nil
}

// forget this Conn, called by TrackedConn.Close()
func (t *ConnTracker) markClosed(c *TrackedConn) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Aggregate TCPInfo from all closed connections
	info, err := c.tcpInfo()
	if err != nil {
		slog.Error("error calling tcpInfo()", slog.Any("error", err))
	} else if info != nil {
		t.tcpInfoSumClosed = addTcpInfo(t.tcpInfoSumClosed, info)
	}

	delete(t.active, c)
	t.closed++
}

// returns the number of active & closed Connections
func (t *ConnTracker) Connections() *ConnTrackerConnections {
	t.mu.Lock()
	defer t.mu.Unlock()

	return &ConnTrackerConnections{
		Active: uint64(len(t.active)),
		Closed: t.closed,
	}
}

//nolint:unused
func (t *ConnTracker) startConnStats() {
	go func() {
		ticker := time.NewTicker(2000 * time.Millisecond)
		defer ticker.Stop()

		for {
			<-ticker.C

			tcpInfo, _ := t.TcpInfo()

			slog.Info("tcpinfo total", slog.Any("tcpinfo", tcpInfo))
		}
	}()
}

func (t *ConnTracker) TcpInfo() (*unix.TCPInfo, error) {
	tcpInfoSum := &unix.TCPInfo{}
	var errs []error

	t.Monkey(func(active map[*TrackedConn]struct{}) {
		for conn := range active {
			tcpInfo, err := conn.tcpInfo()
			if err != nil {
				errs = append(errs, gherrors.Wrap(err, "getConnTcpInfo()"))
				continue
			}

			// Aggregate TCPInfo from all live connections
			tcpInfoSum = addTcpInfo(tcpInfoSum, tcpInfo)
		}

		// access t.tcpInfoSumClosed while Monkey() is holding the lock
		tcpInfoSum = addTcpInfo(t.tcpInfoSumClosed, tcpInfoSum)
	})

	if len(errs) > 0 {
		return tcpInfoSum, errors.Join(errs...)
	}

	return tcpInfoSum, nil
}

func addTcpInfo(a, b *unix.TCPInfo) *unix.TCPInfo {
	infoSum := *a

	infoSum.Bytes_acked += b.Bytes_acked
	infoSum.Bytes_received += b.Bytes_received
	infoSum.Bytes_retrans += b.Bytes_retrans
	infoSum.Bytes_sent += b.Bytes_sent
	infoSum.Dsack_dups += b.Dsack_dups
	infoSum.Fackets += b.Fackets
	infoSum.Lost += b.Lost
	infoSum.Rcv_ooopack += b.Rcv_ooopack
	infoSum.Reord_seen += b.Reord_seen
	infoSum.Retrans += b.Retrans
	infoSum.Sacked += b.Sacked
	infoSum.Total_retrans += b.Total_retrans

	return &infoSum
}

type TrackableConn interface {
	net.Conn
	syscall.Conn
}

// Wraps a net.Conn to allow tracking state.
type TrackedConn struct {
	net.Conn
	tracker *ConnTracker
	once    sync.Once
}

var _ TrackableConn = (*TrackedConn)(nil)

func newTrackedConn(c TrackableConn, tracker *ConnTracker) *TrackedConn {
	return &TrackedConn{
		Conn:    c,
		tracker: tracker,
	}
}

// impliments syscall.Conn interface
func (tc *TrackedConn) SyscallConn() (syscall.RawConn, error) {
	if sc, ok := tc.Conn.(syscall.Conn); ok {
		return sc.SyscallConn()
	}
	return nil, gherrors.New("unable to cast net.Conn to syscall.Conn")
}

// Notify the tracker that the Conn has been closed
func (tc *TrackedConn) Close() error {
	tc.once.Do(func() { tc.tracker.markClosed(tc) })
	return tc.Conn.Close()
}

func (tc *TrackedConn) setPacingRate(pace uint64) error {
	rc, err := tc.SyscallConn()
	if err != nil {
		return gherrors.Wrap(err, "unable to obtain syscall.RawConn")
	}
	var operr error
	if err := rc.Control(func(fd uintptr) {
		// syscall.SetsockoptInt64 doesn't exist
		operr = syscall.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_MAX_PACING_RATE, int(pace)) //gosec:disable G115
	}); err != nil {
		return err
	}
	if operr != nil {
		return operr
	}

	return nil
}

func (tc *TrackedConn) tcpInfo() (*unix.TCPInfo, error) {
	rc, err := tc.SyscallConn()
	if err != nil {
		return nil, gherrors.Wrap(err, "unable to obtain syscall.RawConn")
	}
	var operr error
	var info *unix.TCPInfo
	if err := rc.Control(func(fd uintptr) {
		info, operr = unix.GetsockoptTCPInfo(int(fd), unix.IPPROTO_TCP, unix.TCP_INFO)
	}); err != nil {
		return nil, gherrors.Wrap(err, "unable to get TCP_INFO")
	}
	if operr != nil {
		return nil, gherrors.Wrap(operr, "unable to get TCP_INFO")
	}

	return info, nil
}
