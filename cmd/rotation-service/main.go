package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"

	"example.com/infrai-key-rotation/rotation"
)

func main() {
	client, err := rotation.NewClient(os.Getenv("INFRAI_BASE_URL"), os.Getenv("INFRAI_API_KEY"), nil)
	if err != nil {
		log.Fatal(err)
	}
	drill := rotation.NewDrill(client)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	})
	mux.HandleFunc("POST /rotation-drills", func(w http.ResponseWriter, r *http.Request) {
		var input rotation.DrillInput
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
			return
		}
		result, err := drill.Run(r.Context(), input)
		if err != nil {
			var apiErr *rotation.APIError
			if errors.As(err, &apiErr) && apiErr.Status >= 400 && apiErr.Status < 500 {
				writeJSON(w, apiErr.Status, map[string]string{"error": apiErr.Error()})
				return
			}
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, result)
	})

	address := os.Getenv("LISTEN_ADDR")
	if address == "" {
		address = ":8080"
	}
	log.Printf("rotation service listening on %s", address)
	log.Fatal(http.ListenAndServe(address, mux))
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("encode response: %v", err)
	}
}
