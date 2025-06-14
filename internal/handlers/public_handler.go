package handlers

import(
	"net/http"
	"log"
	"fmt"
)

func PublicHandler(w http.ResponseWriter, r *http.Request) {
	log.Println("Request to public endpoint: /api/public")
	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprintf(w, "Hello, this is public content! (From /api/public)")
}
