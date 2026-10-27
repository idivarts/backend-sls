package main

import (
	"context"
	"fmt"

	"github.com/idivarts/backend-sls/pkg/openrouter"
)

// verify re-reads the registry the way the backend does and resolves the design
// task for every plan, so the check exercises the real path rather than the
// document we just wrote.
func verify(ctx context.Context) {
	openrouter.EnsureRegistry(ctx)
	fmt.Println("\nresolving design through the real registry:")
	for _, p := range []openrouter.Plan{
		openrouter.PlanFree, openrouter.PlanPro, openrouter.PlanTeam, openrouter.PlanAgency,
	} {
		model, locked := openrouter.Resolve(openrouter.TaskDesign, p, "")
		status := model
		if locked {
			status = "LOCKED"
		}
		fmt.Printf("  %-8s -> %s\n", p, status)
	}
}
