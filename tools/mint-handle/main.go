// Command mint-handle allocates a unique decomk handle for a new coordination record.
//
// Usage:
//
//	mint-handle [-w 1|2] [-r REPO_ROOT] [-n] [-s SEED]
//
// The command prints a single proquint handle, such as "vapoj". The caller
// prefixes it with the record kind, for example TODO-vapoj or TE-vapoj.
//
// Intent: Provide one local, collision-checked minting path for new TODO, TE,
// DI records. Source: DI-puhon
package main

import (
	"crypto/sha256"
	"encoding/binary"
	"flag"
	"fmt"
	"os"
	"time"
)

func main() {
	var (
		width    = flag.Int("w", 1, "handle width: 1 (proquint-1) or 2 (proquint-2)")
		repoRoot = flag.String("r", ".", "decomk repo root")
		dryRun   = flag.Bool("n", false, "dry-run (use seed once, no clock retries)")
		seed     = flag.Int64("s", 0, "entropy seed override; 0 means use clock")
	)
	flag.Parse()

	if *width != 1 && *width != 2 {
		fmt.Fprintf(os.Stderr, "mint-handle: -w must be 1 or 2, got %d\n", *width)
		os.Exit(2)
	}

	corpus, err := scanCorpus(*repoRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mint-handle: %v\n", err)
		os.Exit(1)
	}

	handle, err := mint(*width, corpus, *seed, *dryRun)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mint-handle: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(handle)
}

// mint returns a fresh handle that is not present in the scanned corpus. It
// folds a time-derived seed through SHA-256, then retries if the candidate is
// already owned by a local record.
func mint(width int, corpus map[string]string, seed int64, dryRun bool) (string, error) {
	const maxAttempts = 1_000_000
	for attempt := 0; attempt < maxAttempts; attempt++ {
		var ns int64
		if attempt == 0 && seed != 0 {
			ns = seed
		} else {
			ns = time.Now().UnixNano()
		}
		var buf [8]byte
		binary.BigEndian.PutUint64(buf[:], uint64(ns))
		sum := sha256.Sum256(buf[:])

		var candidate string
		switch width {
		case 1:
			candidate = proquint1FromBytes(sum[:2])
		case 2:
			candidate = proquint2FromBytes(sum[:4])
		}

		if _, taken := corpus[candidate]; !taken {
			return candidate, nil
		}
		if dryRun {
			return "", fmt.Errorf("dry-run: first attempt %q collides with corpus", candidate)
		}
		time.Sleep(time.Microsecond)
	}
	return "", fmt.Errorf("exhausted %d attempts; corpus may be saturated (%d handles in use)", maxAttempts, len(corpus))
}
