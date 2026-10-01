package languages

//go:generate go -C tools/registrygen run .
//go:generate go -C tools/registrygen run . -heuristics -source heuristics.yml -out ../../heuristics_generated.go

type nameEntry struct {
	name     string
	language Language
}

type pathEntry struct {
	name      string
	languages Set
}

func lookupPath(entries []pathEntry, name string) Set {
	lo, hi := 0, len(entries)
	for lo < hi {
		mid := lo + (hi-lo)>>1
		if name == entries[mid].name {
			return entries[mid].languages
		}
		if name < entries[mid].name {
			hi = mid
		} else {
			lo = mid + 1
		}
	}
	return Set{}
}
