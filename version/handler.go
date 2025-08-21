package version

import (
	"encoding/json"
	"net/http"

	"github.com/lsst-dm/deliverator/conf"
)

type VersionHandler struct {
	conf *conf.S3ndConf
}

type VersionInfo struct {
	Version string            `json:"version" example:"0.0.0"`
	Config  map[string]string `json:"config,omitempty"`
} // @name VersionInfo

func NewHandler(conf *conf.S3ndConf) *VersionHandler {
	return &VersionHandler{conf: conf}
}

// @summary      report service version and configuration
// @tags         version
// @produce      json
// @success      200  {object}  VersionInfo
// @router       /version [get]
func (h *VersionHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	info := &VersionInfo{
		Version: Version,
		Config:  h.conf.ToMap(),
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(info)
}
