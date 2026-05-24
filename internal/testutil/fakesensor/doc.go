// Package fakesensor is the source of a cross-platform Go binary used as
// a stand-in for real sensor commands in executor + lifecycle tests.
// Test packages compile this binary from TestMain into a temp directory
// and invoke it via the sensor's run: field.
//
// Subcommands:
//
//	signal pass    [--observation-key K]   Emit one pass Signal.
//	signal fail    [--summary S]           Emit one fail Signal + heal hint.
//	stream N       [--interval D]          Emit N pass Signals, optional sleep.
//	crash          [--exit-code C] [--stderr S]
//	                                       Print S to stderr, exit C.
//	watch          [--emit K]... [--interval D]
//	                                       Emit one Signal per --emit value,
//	                                       then loop until SIGTERM / SIGKILL.
//	sleep D                                Sleep D, emit nothing.
package fakesensor
