package main

// Intent: Generate canonical confrepo starter files from the same templates
// `decomk init-conf` embeds, so checked-in examples stay synchronized with
// command output.
// Source: DI-013-20260422-110500 (TODO/013)
//
//go:generate go run ../confrepogen -repo-root ../..
