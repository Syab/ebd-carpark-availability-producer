package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/confluentinc/confluent-kafka-go/kafka"
)

type Results struct {
	Items []Item `json:"items"`
}

type Item struct {
	Timestamp   string         `json:"timestamp"`
	CarparkData []CarparkDatum `json:"carpark_data"`
}

type CarparkDatum struct {
	CarparkInfo    []CarparkInfo `json:"carpark_info"`
	CarparkNumber  string        `json:"carpark_number"`
	UpdateDatetime string        `json:"update_datetime"`
}

type CarparkInfo struct {
	TotalLots     string `json:"total_lots"`
	LotType       string `json:"lot_type"`
	LotsAvailable string `json:"lots_available"`
}

func main() {
	httpPostUrl := "https://api.data.gov.sg/v1/transport/carpark-availability"
	topic := "carpark-availability"

	response, err := sendGetRequest(httpPostUrl, nil)
	if err != nil {
		log.Fatalf("error sending POST request to response url: %v", err)
	}

	var result Results
	err = json.Unmarshal([]byte(response), &result)
	if err != nil {
		log.Fatalf("error unmarshalling result: %v", err)
	}

	fmt.Println(len(result.Items[0].CarparkData))

	//kafka producer
	p, err := kafka.NewProducer(&kafka.ConfigMap{
		"bootstrap.servers": "localhost:9092",
		"client.id":         "1",
		"acks":              "all"})

	if err != nil {
		fmt.Printf("Failed to create producer: %s\n", err)
		os.Exit(1)
	}

	// Go-routine to handle message delivery reports and
	// possibly other event types (errors, stats, etc)
	go func() {
		for e := range p.Events() {
			switch ev := e.(type) {
			case *kafka.Message:
				if ev.TopicPartition.Error != nil {
					fmt.Printf("Failed to deliver message: %v\n", ev.TopicPartition)
				} else {
					fmt.Printf("Successfully produced record to topic %s partition [%d] @ offset %v\n",
						*ev.TopicPartition.Topic, ev.TopicPartition.Partition, ev.TopicPartition.Offset)
				}
			}
		}
	}()

	for _, r := range result.Items[0].CarparkData {
		jDoc, err := json.Marshal(r)
		if err != nil {
			log.Fatalf("Unexpected error: %s", err)
		}

		//kafka produce
		p.Produce(&kafka.Message{
			TopicPartition: kafka.TopicPartition{Topic: &topic, Partition: kafka.PartitionAny},
			Key:            []byte(fmt.Sprintf("%v-%v", time.Now().Format("2006-01-02 15:04"), r.CarparkNumber)),
			Value:          jDoc,
		}, nil)
	}

	p.Flush(15 * 1000)
	p.Close()
	fmt.Println("done sending to kafka")

}

func sendGetRequest(endpoint string, body []byte) (response []byte, err error) {
	req, err := http.NewRequest("GET", endpoint, bytes.NewBuffer(body))
	if err != nil {
		log.Fatalf("err while sending GET request: %v", err)
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	resp_body, _ := ioutil.ReadAll(resp.Body)
	return resp_body, nil
}
