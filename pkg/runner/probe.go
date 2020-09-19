package runner

import (
	"net"
	"sync"
	"syscall"
	"time"

	"github.com/projectdiscovery/naabu/pkg/scan"
)

func (r *Runner) pingprobes(ip string) bool {
	result := true

	if r.options.IcmpEchoProbe {
		result = result && scan.PingIcmpEchoRequest(ip, time.Duration(r.options.Timeout)*time.Millisecond)
	}
	if r.options.IcmpTimestampProbe {
		result = result || scan.PingIcmpTimestampRequest(ip, time.Duration(r.options.Timeout)*time.Millisecond)
	}

	return result
}

func (r *Runner) pingprobesasync(ip string) {
	r.scanner.EnqueueICMP(ip, scan.ICMP_ECHO_REQUEST)
	r.scanner.EnqueueICMP(ip, scan.ICMP_TIMESTAMP_REQUEST)
}

func (r *Runner) synprobes(ip string) bool {
	for p := range r.scanner.SynProbesPorts {
		ok, err := scan.ConnectPort(ip, p, time.Duration(r.options.Timeout)*time.Millisecond)
		if ok || hasRefusedConnection(err) {
			return true
		}
	}

	return false
}

func (r *Runner) synprobesasync(ip string) {
	for p := range r.scanner.SynProbesPorts {
		r.scanner.EnqueueTCP(ip, p, scan.SYN)
	}
}

func (r *Runner) ackprobes(ip string) bool {
	for p := range r.scanner.AckProbesPorts {
		ok, err := r.scanner.ACKPort(ip, p, time.Duration(r.options.Timeout)*time.Millisecond)
		if ok && err == nil {
			return true
		}
	}
	return false
}

func (r *Runner) ackprobesasync(ip string) {
	for p := range r.scanner.AckProbesPorts {
		r.scanner.EnqueueTCP(ip, p, scan.ACK)
	}
}

func hasRefusedConnection(err error) bool {
	// no error
	if err == nil {
		return false
	}

	// timeout
	if netError, ok := err.(net.Error); ok && netError.Timeout() {
		return false
	}

	switch t := err.(type) {
	case *net.OpError:
		// Unknown host
		if t.Op == "dial" {
			return false
		}

		// Connection refused
		if t.Op == "read" {
			return true
		}

	case syscall.Errno:
		// Connection refused
		if t == syscall.ECONNREFUSED {
			return true
		}
	}

	return false
}

func (r *Runner) ProbeOrSkip() {
	r.scanner.State = scan.Probe
	if r.options.NoProbe {
		return
	}
	// root is required
	if !isRoot() {
		return
	}

	var swg sync.WaitGroup
	limiter := time.Tick(time.Second / time.Duration(r.options.Rate))
	for ip := range r.scanner.Targets {
		<-limiter
		swg.Add(1)
		go func(ip string) {
			defer swg.Done()
			r.pingprobesasync(ip)
			r.synprobesasync(ip)
			r.ackprobesasync(ip)
		}(ip)
	}
	swg.Wait()
}
