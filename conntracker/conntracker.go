package conntracker

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"sync"
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
	active           map[net.Conn]struct{}
	tcpInfoSumClosed *unix.TCPInfo
	closed           uint64 // number of closed connections
}

type ConnTrackerConnections struct {
	Active uint64 // number of active connections
	Closed uint64 // number of closed connections
}

func NewConnTracker(d *net.Dialer) *ConnTracker {
	t := &ConnTracker{
		dialer:           d,
		active:           make(map[net.Conn]struct{}),
		tcpInfoSumClosed: &unix.TCPInfo{},
	}

	// enable periodic stats logging -- useful for debugging
	// t.startConnStats()

	return t
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
func (t *ConnTracker) Monkey(do func(active map[net.Conn]struct{})) {
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

	t.mu.Lock()
	t.active[conn] = struct{}{}
	t.mu.Unlock()

	// Wrap the conn so we can see when the request finishes and
	// the transport calls Close().
	tConn := newTrackedConn(conn, t)

	return tConn, nil
}

// forget this Conn, called by trackedConn.Close()
func (t *ConnTracker) markClosed(c net.Conn) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Aggregate TCPInfo from all closed connections
	info, err := getConnTcpInfo(c)
	if err != nil {
		slog.Error("error calling getConnTcpInfo()", slog.Any("error", err))
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

			tcpInfo, _ := t.GetTcpInfo()

			slog.Info("tcpinfo total", slog.Any("tcpinfo", tcpInfo))
		}
	}()
}

func (t *ConnTracker) GetTcpInfo() (*unix.TCPInfo, error) {
	tcpInfoSum := &unix.TCPInfo{}
	var errs []error

	t.Monkey(func(active map[net.Conn]struct{}) {
		for conn := range active {
			tcpInfo, err := getConnTcpInfo(conn)
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

func getConnTcpInfo(conn net.Conn) (*unix.TCPInfo, error) {
	rc, err := getRawConn(conn)
	if err != nil {
		return nil, err
	}

	info, err := getRawConnTcpInfo(rc)
	if err != nil {
		return nil, err
	}

	return info, nil
}

func getRawConn(conn net.Conn) (syscall.RawConn, error) {
	sc, ok := conn.(syscall.Conn)
	if !ok {
		return nil, errors.New("unable to cast net.Conn to syscall.Conn")
	}

	rc, err := sc.SyscallConn()
	if err != nil {
		return nil, gherrors.Wrap(err, "unable to obtain syscall.RawConn from net.Conn")
	}

	return rc, nil
}

func getRawConnTcpInfo(conn syscall.RawConn) (*unix.TCPInfo, error) {
	// https://pkg.go.dev/syscall#RawConn
	var operr error
	var info *unix.TCPInfo
	if err := conn.Control(func(fd uintptr) {
		info, operr = unix.GetsockoptTCPInfo(int(fd), unix.IPPROTO_TCP, unix.TCP_INFO)
	}); err != nil {
		return nil, gherrors.Wrap(err, "unable to get TCP_INFO")
	}
	if operr != nil {
		return nil, gherrors.Wrap(operr, "unable to get TCP_INFO")
	}

	return info, nil
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

// Wraps a net.Conn to allow tracking state.
type trackedConn struct {
	net.Conn
	tracker *ConnTracker
	once    sync.Once
}

func newTrackedConn(c net.Conn, tracker *ConnTracker) *trackedConn {
	return &trackedConn{
		Conn:    c,
		tracker: tracker,
	}
}

// Notify the tracker that the Conn has been closed
func (tc *trackedConn) Close() error {
	tc.once.Do(func() { tc.tracker.markClosed(tc.Conn) })
	return tc.Conn.Close()
}
