package collector

import (
	"log/slog"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/lsst-dm/s3nd/upload"
)

type S3ndCollector struct {
	handler *upload.S3ndHandler
	descs   map[string]*prometheus.Desc
}

func NewS3ndCollector(handler *upload.S3ndHandler) prometheus.Collector {
	descs := map[string]*prometheus.Desc{
		"bytes_acked":    prometheus.NewDesc("tcp_info_bytes_acked", "tcpi_bytes_acked from tcp_info", []string{"endpoint_url"}, nil),
		"bytes_received": prometheus.NewDesc("tcp_info_bytes_received", "tcpi_bytes_received from tcp_info", []string{"endpoint_url"}, nil),
		"bytes_retrans":  prometheus.NewDesc("tcp_info_bytes_retrans", "tcpi_bytes_retrans from tcp_info", []string{"endpoint_url"}, nil),
		"bytes_sent":     prometheus.NewDesc("tcp_info_bytes_sent", "tcpi_bytes_sent from tcp_info", []string{"endpoint_url"}, nil),
		"dsack_dups":     prometheus.NewDesc("tcp_info_dsack_dups", "tcpi_dsack_dups from tcp_info", []string{"endpoint_url"}, nil),
		"fackets":        prometheus.NewDesc("tcp_info_fackets", "tcpi_fackets from tcp_info", []string{"endpoint_url"}, nil),
		"lost":           prometheus.NewDesc("tcp_info_lost", "tcpi_lost from tcp_info", []string{"endpoint_url"}, nil),
		"rcv_ooopack":    prometheus.NewDesc("tcp_info_rcv_ooopack", "tcpi_rcv_ooopack from tcp_info", []string{"endpoint_url"}, nil),
		"reord_seen":     prometheus.NewDesc("tcp_info_reord_seen", "tcpi_reord_seen from tcp_info", []string{"endpoint_url"}, nil),
		"retrans":        prometheus.NewDesc("tcp_info_retrans", "tcpi_retrans from tcp_info", []string{"endpoint_url"}, nil),
		"sacked":         prometheus.NewDesc("tcp_info_sacked", "tcpi_sacked from tcp_info", []string{"endpoint_url"}, nil),
		"total_retrans":  prometheus.NewDesc("tcp_info_total_retrans", "tcpi_total_retrans from tcp_info", []string{"endpoint_url"}, nil),
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

	conf := c.handler.Conf()
	if conf == nil {
		slog.Error("configuration is nil")
		return
	}
	if conf.EndpointUrl == nil {
		slog.Error("EndpointUrl is nil in configuration")
		return
	}
	eUrl := *conf.EndpointUrl
	metrics := []struct {
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
	}

	for _, metric := range metrics {
		ch <- prometheus.MustNewConstMetric(c.descs[metric.name], prometheus.CounterValue, metric.value, eUrl)
	}
}
