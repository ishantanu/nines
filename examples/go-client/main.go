// Preview a typed SLO plan; NINES_APPLY=true applies it to the configured tenant.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/client"
)

func main() {
	c, err := client.New(client.Config{URL: os.Getenv("NINES_URL"), Tenant: os.Getenv("NINES_TENANT"), Token: os.Getenv("API_TOKEN")})
	if err != nil {
		log.Fatal(err)
	}
	slo := v1.NewSLO("api-availability", "production", v1.SLOSpec{DisplayName: "API availability", Service: "gateway", Target: .999, WindowSeconds: 86400, StepSeconds: 60, GoodQuery: `sum(increase(http_requests_total{code!~"5.."}[1m]))`, TotalQuery: `sum(increase(http_requests_total[1m]))`})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	plan, err := c.Plan(ctx, slo)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%s %s (expected revision %d)\n", plan.Action, slo.Metadata.Name, plan.ExpectedRevision)
	if os.Getenv("NINES_APPLY") != "true" {
		fmt.Println("Review the plan; set NINES_APPLY=true to apply.")
		return
	}
	revision, err := c.Apply(ctx, slo, plan.ExpectedRevision)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Applied revision %d, digest %s\n", revision.Revision, revision.Digest)
}
