package agent

import "testing"

func TestBudgetExhaustion(t *testing.T) {
	// unlimited never exhausts
	if NewBudget(0, 0).Exhausted(Spend{Tokens: 1 << 40, Turns: 1 << 20}) {
		t.Fatal("unlimited budget must never exhaust")
	}
	// tokens: budget 100k, reserve 20k floor -> exhausted when remaining <= 20k
	b := NewBudget(100_000, 0)
	if b.Exhausted(Spend{Tokens: 79_999}) {
		t.Fatal("79k spent of 100k must not exhaust (remaining 20k1 > reserve 20k)")
	}
	if !b.Exhausted(Spend{Tokens: 80_000}) {
		t.Fatal("80k spent of 100k must exhaust (remaining 20k <= reserve 20k)")
	}
	// turns: budget 10, reserve max(0,2)=2 -> exhausted at remaining 2
	bt := NewBudget(0, 10)
	if bt.Exhausted(Spend{Turns: 7}) || !bt.Exhausted(Spend{Turns: 8}) {
		t.Fatalf("turn exhaustion boundary wrong: %v", bt.Remaining(Spend{Turns: 8}))
	}
	// one capped dimension does not constrain the other
	mixed := NewBudget(100_000, 0)
	if mixed.Exhausted(Spend{Tokens: 1_000, Turns: 9_999_999}) {
		t.Fatal("turns spend must not exhaust a tokens-only budget")
	}
	// over-spend clamps at zero, still exhausted
	if !b.Exhausted(Spend{Tokens: 500_000}) {
		t.Fatal("over-spend must stay exhausted")
	}
	// margin of a small budget clamps to the budget itself: a budget smaller
	// than the reserve floor cannot afford a wrap-up round, so it exhausts as
	// soon as it is fully spent.
	small := NewBudget(5_000, 0)
	if got := WrapupMargin(small).Tokens; got != 5_000 {
		t.Fatalf("margin of a 5000-token budget clamps to 5000, got %d", got)
	}
	tiny := NewBudget(100, 0)
	if got := WrapupMargin(tiny).Tokens; got != 100 {
		t.Fatalf("margin of a 100-token budget clamps to 100, got %d", got)
	}
}
