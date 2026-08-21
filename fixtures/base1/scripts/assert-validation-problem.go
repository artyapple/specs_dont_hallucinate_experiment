package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"os"
)

type problem struct {
	Type   string `json:"type"`
	Title  string `json:"title"`
	Status int    `json:"status"`
	Detail string `json:"detail"`
}

func main() {
	if len(os.Args) != 3 {
		panic("usage: assert-validation-problem <body> <content-type>")
	}
	mediaType, _, err := mime.ParseMediaType(os.Args[2])
	if err != nil || mediaType != "application/problem+json" {
		panic(fmt.Sprintf("Content-Type = %q, want application/problem+json", os.Args[2]))
	}
	file, err := os.Open(os.Args[1])
	if err != nil {
		panic(err)
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	var got problem
	if err := decoder.Decode(&got); err != nil {
		panic(err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		panic("response contains multiple JSON values")
	}
	want := problem{"urn:problem:validation", "Validation failed", 400, "The request is invalid."}
	if got != want {
		panic(fmt.Sprintf("Problem Details = %+v, want %+v", got, want))
	}
}
