# jev-go

A small, zero-dependency Go SDK for the [TypeSafe AI Jev API](https://docs.typesafe.ai/api).

## Install

```bash
go get github.com/havlan/jev-go
```

## Use

```go
package main

import (
	"context"
	"fmt"
	"log"

	jev "github.com/havlan/jev-go"
)

func main() {
	client, err := jev.NewFromEnv() // reads TYPESAFE_API_KEY
	if err != nil {
		log.Fatal(err)
	}

	response, err := client.SystemOne(context.Background(), "Help! My payouts have failed for three days.", jev.Questions{
		"urgent": jev.Noul("Does this message express urgency?"),
		"department": jev.Choice("Which team should handle this?", map[string]string{
			"billing":   "Payments, invoices, and refunds",
			"technical": "Bugs, outages, and integrations",
		}),
		"frustration": jev.Score("How frustrated is the customer?", "calm", "frustrated", "very angry"),
	})
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(response.Answers["urgent"].Noul)
	fmt.Println(response.Answers["department"].Choice)
	fmt.Println(response.Answers["frustration"].Score)
}
```

`SystemOne` uses `jev-latest`, retries `429` and `529` responses, and accepts string, object, or array state. Use `New(jev.Config{...})` to override the model, endpoint, HTTP client, or retry count.

## Test

```bash
go test ./...
```
