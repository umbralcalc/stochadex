package stochadex

type TimestepFunction interface {
	Iterate(timestepsHistory *TimestepsHistory) *TimestepsHistory
}
