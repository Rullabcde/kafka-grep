// Command seed writes sample data to Kafka topics for testing kafkagrep.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"strings"
	"time"

	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"
)

type record struct {
	Event   string  `json:"event"`
	UserID  string  `json:"user_id"`
	OrderID string  `json:"order_id,omitempty"`
	Amount  float64 `json:"amount,omitempty"`
	Status  string  `json:"status,omitempty"`
}

func main() {
	brokers := flag.String("brokers", "localhost:9092", "Kafka brokers")
	topics := flag.String("topics", "orders,payments,notifications", "comma-separated topics to seed")
	n := flag.Int("n", 1000, "records per topic")
	flag.Parse()

	client, err := kgo.NewClient(kgo.SeedBrokers(*brokers))
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	adm := kadm.NewClient(client)
	if _, err := adm.CreateTopics(context.Background(), 4, 1, nil, splitTopics(*topics)...); err != nil {
		fmt.Printf("create topics: %v\n", err)
	}

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	for _, topic := range splitTopics(*topics) {
		for i := 0; i < *n; i++ {
			rec := record{
				Event:   "order_created",
				UserID:  fmt.Sprintf("user_%d", rng.Intn(10000)),
				OrderID: fmt.Sprintf("order-%d", rng.Intn(1000000)),
				Amount:  float64(rng.Intn(50000)) / 100.0,
				Status:  statuses[rng.Intn(len(statuses))],
			}
			val, _ := json.Marshal(rec)
			if err := client.ProduceSync(context.Background(),
				&kgo.Record{Topic: topic, Value: val}).FirstErr(); err != nil {
				log.Fatalf("produce %s: %v", topic, err)
			}
		}
		fmt.Printf("seeded %d records to %s\n", *n, topic)
	}
	fmt.Println("done")
}

var statuses = []string{"CREATED", "PAID", "FAILED", "SHIPPED", "CANCELLED"}

func splitTopics(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
