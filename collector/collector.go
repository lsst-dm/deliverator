package collector

import (
	"log/slog"
	"strconv"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/lsst-dm/s3nd/upload"
)

type S3ndCollector struct {
	handler *upload.S3ndHandler
	descs   map[string]*prometheus.Desc
}

func NewS3ndCollector(handler *upload.S3ndHandler) prometheus.Collector {
	labels := []string{"endpoint_url", "port"}
	descs := map[string]*prometheus.Desc{
		"bytes_acked":    prometheus.NewDesc("s3nd_s3_tcp_info_bytes_acked", "tcpi_bytes_acked from tcp_info", labels, nil),
		"bytes_received": prometheus.NewDesc("s3nd_s3_tcp_info_bytes_received", "tcpi_bytes_received from tcp_info", labels, nil),
		"bytes_retrans":  prometheus.NewDesc("s3nd_s3_tcp_info_bytes_retrans", "tcpi_bytes_retrans from tcp_info", labels, nil),
		"bytes_sent":     prometheus.NewDesc("s3nd_s3_tcp_info_bytes_sent", "tcpi_bytes_sent from tcp_info", labels, nil),
		"dsack_dups":     prometheus.NewDesc("s3nd_s3_tcp_info_dsack_dups", "tcpi_dsack_dups from tcp_info", labels, nil),
		"fackets":        prometheus.NewDesc("s3nd_s3_tcp_info_fackets", "tcpi_fackets from tcp_info", labels, nil),
		"lost":           prometheus.NewDesc("s3nd_s3_tcp_info_lost", "tcpi_lost from tcp_info", labels, nil),
		"rcv_ooopack":    prometheus.NewDesc("s3nd_s3_tcp_info_rcv_ooopack", "tcpi_rcv_ooopack from tcp_info", labels, nil),
		"reord_seen":     prometheus.NewDesc("s3nd_s3_tcp_info_reord_seen", "tcpi_reord_seen from tcp_info", labels, nil),
		"retrans":        prometheus.NewDesc("s3nd_s3_tcp_info_retrans", "tcpi_retrans from tcp_info", labels, nil),
		"sacked":         prometheus.NewDesc("s3nd_s3_tcp_info_sacked", "tcpi_sacked from tcp_info", labels, nil),
		"total_retrans":  prometheus.NewDesc("s3nd_s3_tcp_info_total_retrans", "tcpi_total_retrans from tcp_info", labels, nil),
		"uploads":        prometheus.NewDesc("s3nd_uploads", "number of active uploads", labels, nil),
		"conn_active":    prometheus.NewDesc("s3nd_s3_tcp_conn_active", "number of active tcp connections to the endpoint", labels, nil),
		"conn_closed":    prometheus.NewDesc("s3nd_s3_tcp_conn_closed", "number of tcp connections to the endpoint which have been closed", labels, nil),
	}
	return &S3ndCollector{handler: handler, descs: descs}
}

func (c *S3ndCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, d := range c.descs {
		ch <- d
	}
}

func (c *S3ndCollector) Collect(ch chan<- prometheus.Metric) {
	tcpInfo, err := c.handler.ConnTracker().GetTcpInfo()
	if err != nil {
		slog.Error("failed to get aggregate TCP info", "error", err)
		return
	}
	counts := c.handler.ConnTracker().Connections()

	conf := c.handler.Conf()
	if conf == nil {
		slog.Error("configuration is nil")
		return
	}
	if conf.EndpointUrl == nil {
		slog.Error("EndpointUrl is nil in configuration")
		return
	}
	if conf.Port == nil {
		slog.Error("Port is nil in configuration")
		return
	}

	labels := []string{*conf.EndpointUrl, strconv.Itoa(*conf.Port)}

	// counter(s)
	counters := []struct {
		name  string
		value float64
	}{
		{"bytes_acked", float64(tcpInfo.Bytes_acked)},
		{"bytes_received", float64(tcpInfo.Bytes_received)},
		{"bytes_retrans", float64(tcpInfo.Bytes_retrans)},
		{"bytes_sent", float64(tcpInfo.Bytes_sent)},
		{"dsack_dups", float64(tcpInfo.Dsack_dups)},
		{"fackets", float64(tcpInfo.Fackets)},
		{"lost", float64(tcpInfo.Lost)},
		{"rcv_ooopack", float64(tcpInfo.Rcv_ooopack)},
		{"reord_seen", float64(tcpInfo.Reord_seen)},
		{"retrans", float64(tcpInfo.Retrans)},
		{"sacked", float64(tcpInfo.Sacked)},
		{"total_retrans", float64(tcpInfo.Total_retrans)},
		{"conn_closed", float64(counts.Closed)},
	}

	for _, metric := range counters {
		ch <- prometheus.MustNewConstMetric(c.descs[metric.name], prometheus.CounterValue, metric.value, labels...)
	}

	// gauge(s)
	gauges := []struct {
		name  string
		value float64
	}{
		{"uploads", float64(c.handler.ParallelUploads().GetCount())},
		{"conn_active", float64(counts.Active)},
	}

	for _, metric := range gauges {
		ch <- prometheus.MustNewConstMetric(c.descs[metric.name], prometheus.GaugeValue, metric.value, labels...)
	}
}
