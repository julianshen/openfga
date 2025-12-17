package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/openfga/go-sdk/client"
)

const (
	NumStores = 100000
	BatchSize = 100
)

type Target struct {
	Name string
	URL  string
}

func main() {
	targets := []Target{
		{"Postgres", "http://localhost:8081"},
		{"MySQL", "http://localhost:8082"},
		{"Valkey", "http://localhost:8083"},
	}

	var wg sync.WaitGroup
	for _, t := range targets {
		wg.Add(1)
		go func(target Target) {
			defer wg.Done()
			seed(target)
		}(t)
	}
	wg.Wait()
}

func seed(target Target) {
	fmt.Printf("[%s] Starting seed of %d stores...\n", target.Name, NumStores)
	start := time.Now()

	fgaClient, err := client.NewSdkClient(&client.ClientConfiguration{
		ApiUrl: target.URL,
	})
	if err != nil {
		log.Fatalf("[%s] Failed to create client: %v", target.Name, err)
	}

	sem := make(chan struct{}, 20) // Limit concurrency per target
	var seedWg sync.WaitGroup

	for i := 0; i < NumStores; i++ {
		seedWg.Add(1)
		sem <- struct{}{}
		go func(idx int) {
			defer seedWg.Done()
			defer func() { <-sem }()

			_, err := fgaClient.CreateStore(context.Background()).Body(client.ClientCreateStoreRequest{
				Name: fmt.Sprintf("store-%s-%d", target.Name, idx),
			}).Execute()
			if err != nil {
				log.Printf("[%s] Failed to create store %d: %v", target.Name, idx, err)
			}
		}(i)

		if i%1000 == 0 && i > 0 {
			fmt.Printf("[%s] Seeded %d/%d...\n", target.Name, i, NumStores)
		}
	}
	seedWg.Wait()
	fmt.Printf("[%s] Completed seeding in %v\n", target.Name, time.Since(start))
}
