package main

import "fmt"

//go:generate go run ./cmd/genregen -in ../src/genre.tsv -out genres_generated.go

func genreDisplayName(id int) string {
	if n, ok := genreNameByID(id); ok {
		return n
	}
	return fmt.Sprintf("genre:%d", id)
}
