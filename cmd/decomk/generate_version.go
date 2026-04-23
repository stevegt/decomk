package main

// Intent: Keep runtime `decomk version` output synchronized with the checked-in
// VERSION file so source builds report the same release identifier used by tags.
// Source: DI-001-20260423-204251 (TODO/001)
//
//go:generate go run ../versiongen -repo-root ../..
