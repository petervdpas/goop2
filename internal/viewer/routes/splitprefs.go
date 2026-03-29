package routes

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"

	"github.com/petervdpas/goop2/internal/util"
)

func splitPrefsPath(d Deps) string {
	return filepath.Join(filepath.Dir(d.CfgPath), "split-prefs.json")
}

func loadSplitPrefs(d Deps) map[string]float64 {
	data, err := os.ReadFile(splitPrefsPath(d))
	if err != nil {
		return nil
	}
	var prefs map[string]float64
	if json.Unmarshal(data, &prefs) != nil {
		return nil
	}
	return prefs
}

func loadSplitPrefsJSON(d Deps) string {
	data, err := os.ReadFile(splitPrefsPath(d))
	if err != nil {
		return "{}"
	}
	var check map[string]float64
	if json.Unmarshal(data, &check) != nil {
		return "{}"
	}
	return string(data)
}

type splitPrefReq struct {
	Key   string  `json:"key"`
	Value float64 `json:"value"`
}

func registerSplitPrefsRoutes(mux *http.ServeMux, d Deps) {
	handlePost(mux, "/api/split-prefs", func(w http.ResponseWriter, r *http.Request, req splitPrefReq) {
		if req.Key == "" {
			http.Error(w, "missing key", http.StatusBadRequest)
			return
		}
		if req.Value < 0 || req.Value > 100 {
			http.Error(w, "value must be 0-100", http.StatusBadRequest)
			return
		}
		prefs := loadSplitPrefs(d)
		if prefs == nil {
			prefs = make(map[string]float64)
		}
		prefs[req.Key] = req.Value
		if err := util.WriteJSONFile(splitPrefsPath(d), prefs); err != nil {
			http.Error(w, "save failed", http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]string{"status": "ok"})
	})
}
