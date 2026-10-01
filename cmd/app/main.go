package main

import (
	"fmt"
	"net/http"

	"AGoTTHT/internal/handler"
)

func main() {
	fs := http.FileServer(http.Dir("static"))
	http.Handle("/static/", http.StripPrefix("/static/", fs))

	// 2. Register application routes
	http.HandleFunc("/api/health", handler.HandleHealth)
	http.HandleFunc("/", handler.HandleHome)

	// 3. Start the server
	fmt.Println("Server running on http://localhost:8080")
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		fmt.Printf("Error starting server: %s\n", err)
	}
}
