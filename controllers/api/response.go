package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/ytalharabaci/phish-and-discover/models"

	log "github.com/ytalharabaci/phish-and-discover/logger"
)

// JSONResponse attempts to set the status code, c, and marshal the given interface, d, into a response that
// is written to the given ResponseWriter.
func JSONResponse(w http.ResponseWriter, d interface{}, c int) {
	dj, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		http.Error(w, "Error creating JSON response", http.StatusInternalServerError)
		log.Error(err)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(c)
	fmt.Fprintf(w, "%s", dj)
}

// idFromVars parses the "id" path variable as an int64. Route patterns
// already constrain this to digits via {id:[0-9]+}, so parsing only fails
// on integer overflow; ok is false in that case and the caller should write
// a 400 response rather than proceeding with a zero-valued ID.
func idFromVars(vars map[string]string) (id int64, ok bool) {
	id, err := strconv.ParseInt(vars["id"], 0, 64)
	if err != nil {
		return 0, false
	}
	return id, true
}

// writeInvalidIDResponse writes the standard 400 response used when the
// "id" path variable cannot be parsed.
func writeInvalidIDResponse(w http.ResponseWriter) {
	JSONResponse(w, models.Response{Success: false, Message: "Invalid ID"}, http.StatusBadRequest)
}
