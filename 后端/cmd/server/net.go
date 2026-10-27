package main

import "net"

func splitAddress(s string) (string, string, error) { return net.SplitHostPort(s) }
func joinAddress(h, p string) string                { return net.JoinHostPort(h, p) }
