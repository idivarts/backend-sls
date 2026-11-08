package studio

import (
	"context"
	"testing"
	"time"
)

// These lock the arithmetic that keeps a timed-out generation from stranding
// its scene. Getting it wrong does not fail loudly — it produces a scene that
// says "Making…" with nothing coming, which is what happened on dev.

func TestTheModelGetsTheRequestMinusTheReserve(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	budgeted, cancelB := modelBudget(ctx)
	defer cancelB()

	deadline, ok := budgeted.Deadline()
	if !ok {
		t.Fatal("the budgeted context should carry a deadline")
	}
	left := time.Until(deadline)
	// 30s request, 6s reserve → ~24s, allowing for the clock moving.
	if left > 24*time.Second || left < 23*time.Second {
		t.Fatalf("model budget = %v, want about 24s", left)
	}
}

// The whole point: there must be time left AFTER the model gives up.
func TestThereIsTimeLeftToRecordAFailure(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	budgeted, cancelB := modelBudget(ctx)
	defer cancelB()
	modelDeadline, _ := budgeted.Deadline()
	requestDeadline, _ := ctx.Deadline()

	if !modelDeadline.Before(requestDeadline) {
		t.Fatal("the model must give up before the request does")
	}
	if gap := requestDeadline.Sub(modelDeadline); gap < 5*time.Second {
		t.Fatalf("only %v left to write the outcome; that is not enough", gap)
	}
}

// A request that arrives already inside the reserve must not start a call it
// cannot finish — that charges for a generation nobody can receive.
func TestARequestWithNoTimeLeftDoesNotStartAGeneration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	budgeted, cancelB := modelBudget(ctx)
	defer cancelB()

	deadline, _ := budgeted.Deadline()
	if left := time.Until(deadline); left > 100*time.Millisecond {
		t.Fatalf("a request inside the reserve gave the model %v", left)
	}
}

// Outside Lambda there is no deadline at all, and the helper must still
// produce a bounded one rather than an unbounded call.
func TestAContextWithNoDeadlineStillGetsABudget(t *testing.T) {
	budgeted, cancelB := modelBudget(context.Background())
	defer cancelB()
	if _, ok := budgeted.Deadline(); !ok {
		t.Fatal("a generation should never be unbounded")
	}
}

// The write that records the outcome must survive the generation's expiry —
// this is precisely what failed on dev, where every post-timeout write reused
// the dead request context and was silently dropped.
func TestBookkeepingSurvivesTheRequestBeingCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	book, cancelBook := bookkeepingCtx(ctx)
	defer cancelBook()

	cancel() // the client gave up, or the generation blew its deadline

	if err := book.Err(); err != nil {
		t.Fatalf("bookkeeping context died with the request: %v", err)
	}
	if _, ok := book.Deadline(); !ok {
		t.Fatal("bookkeeping must still be bounded, or it hangs until the kill")
	}
}
