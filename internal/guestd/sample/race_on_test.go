//go:build race

package sample

// raceEnabled: the race detector slows this code 5 to 10 times, so a
// wall-clock budget measured under it says nothing about a guest.
const raceEnabled = true
