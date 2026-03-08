package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/imdea-software/swiftpaxos/client"
	"github.com/imdea-software/swiftpaxos/config"
	"github.com/imdea-software/swiftpaxos/curp"
	curpht "github.com/imdea-software/swiftpaxos/curp-ht"
	curpho "github.com/imdea-software/swiftpaxos/curp-ho"
	"github.com/imdea-software/swiftpaxos/dlog"
	"github.com/imdea-software/swiftpaxos/master"
	"github.com/imdea-software/swiftpaxos/raft"
	raftht "github.com/imdea-software/swiftpaxos/raft-ht"
	"github.com/imdea-software/swiftpaxos/replica/defs"
	"github.com/imdea-software/swiftpaxos/swift"
)

var (
	confs        = flag.String("config", "", "Deployment config `file` (required)")
	latency      = flag.String("latency", "", "Latency config `file`")
	logFile      = flag.String("log", "", "Path to the log `file`")
	machineAlias = flag.String("alias", "", "An `alias` of this participant")
	machineType  = flag.String("run", "server", "Run a `participant`, which is either a server (or replica), a client or a master")
	protocol     = flag.String("protocol", "", "Protocol to run. Overwrites `protocol` field of the config file")
	quorum       = flag.String("quorum", "", "Quorum config `file`")
)

func main() {
	flag.Parse()

	if *confs == "" {
		flag.Usage()
		os.Exit(1)
	}

	c, err := config.Read(*confs, *machineAlias)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	if *protocol != "" {
		c.Protocol = *protocol
	}
	defs.LatencyConf = *latency

	switch *machineType {
	case "replica":
		fallthrough
	case "server":
		c.MachineType = config.ReplicaMachine
	case "client":
		c.MachineType = config.ClientMachine
	case "master":
		c.MachineType = config.MasterMachine
	default:
		fmt.Println("Unknown participant type")
		flag.Usage()
		os.Exit(1)
	}

	c.Quorum = *quorum

	run(c)
}

func run(c *config.Config) {
	switch c.MachineType {
	case config.MasterMachine:
		runMaster(c)
	case config.ClientMachine:
		runClient(c, true)
	case config.ReplicaMachine:
		runReplica(c, dlog.New(*logFile, true))
	}
}

func runMaster(c *config.Config) {
	m := master.New(len(c.ReplicaAddrs), c.MasterPort, dlog.New(*logFile, true))
	m.Run()
}

func runClient(c *config.Config, verbose bool) {
	// Set protocol-specific config flags BEFORE spawning goroutines
	// to avoid data races on shared *config.Config.
	switch strings.ToLower(c.Protocol) {
	case "curpho":
		c.Fast = true
	case "fastpaxos":
		c.Fast = true
		c.WaitClosest = true
	case "n2paxos":
		c.Fast = true
		c.WaitClosest = true
	case "epaxos":
		c.Leaderless = true
		c.Fast = false
	case "paxos":
		c.WaitClosest = false
		c.Fast = false
	case "raft":
		c.WaitClosest = false
		c.Fast = false
	case "raftht":
		c.WaitClosest = false
		c.Fast = false
	}

	numThreads := c.GetNumClientThreads()

	// Collect metrics and durations from all threads
	allMetrics := make([]*client.HybridMetrics, numThreads)
	allDurations := make([]time.Duration, numThreads)
	var metricsLock sync.Mutex

	var wg sync.WaitGroup
	for i := 0; i < numThreads; i++ {
		wg.Add(1)
		go func(i int) {
			metrics, duration := runSingleClient(c, i, verbose, numThreads)
			metricsLock.Lock()
			allMetrics[i] = metrics
			allDurations[i] = duration
			metricsLock.Unlock()
			wg.Done()
		}(i)
	}
	wg.Wait()

	// Use max duration across all threads (they run in parallel)
	var maxDuration time.Duration
	for _, d := range allDurations {
		if d > maxDuration {
			maxDuration = d
		}
	}

	// Aggregate and print/export metrics
	p := strings.ToLower(c.Protocol)
	if p == "curp" || p == "curpht" || p == "curpho" || p == "raft" || p == "raftht" {
		aggregated := client.AggregateMetrics(allMetrics)
		if numThreads > 1 {
			l := dlog.New(*logFile, verbose)
			l.Printf("Test took %v\n", maxDuration)
			aggregated.Print(l, c.Reqs*numThreads, maxDuration)
		}

		// Export raw latencies for CDF plotting
		latPath := fmt.Sprintf("latencies-%s.json", c.Alias)
		if err := aggregated.ExportLatencies(latPath); err != nil {
			log.Printf("Warning: failed to export latencies to %s: %v", latPath, err)
		}
	}
}

func runSingleClient(c *config.Config, threadIdx int, verbose bool, numThreads int) (*client.HybridMetrics, time.Duration) {
	// Thread 0 uses the main log file, other threads use /dev/null to avoid clutter
	// All operational output goes through thread 0
	var l *dlog.Logger
	if threadIdx == 0 {
		l = dlog.New(*logFile, verbose)
	} else {
		l = dlog.New("/dev/null", false)
	}

	server := c.Proxy.ProxyOf(c.ClientAddrs[c.Alias])
	server = c.ReplicaAddrs[server]
	cl := client.NewClientLog(server, c.MasterAddr, c.MasterPort, c.Fast, c.Leaderless, verbose, l)
	b := client.NewBufferClient(cl, c.Reqs, c.CommandSize, c.Conflicts, c.Writes, int64(c.Key))
	if c.Pipeline {
		b.Pipeline(c.Syncs, int32(c.Pendings))
	}
	// Configure KeyGenerator for Zipf/Uniform key distribution
	if c.KeySpace > 0 {
		keyGen := client.NewKeyGenerator(c.KeySpace, c.ZipfSkew, cl.ClientId)
		b.SetKeyGenerator(keyGen)
	}
	if err := b.Connect(); err != nil {
		log.Fatal(err)
	}
	if p := strings.ToLower(c.Protocol); p == "swiftpaxos" {
		cl := swift.NewClient(b, len(c.ReplicaAddrs))
		if cl == nil {
			return nil, 0
		}
		cl.Loop()
		return nil, 0
	} else if p == "curp" {
		cls := []string{}
		for a := range c.ClientAddrs {
			cls = append(cls, a)
		}
		sort.Slice(cls, func(i, j int) bool {
			return cls[i] < cls[j]
		})
		pclients := c.GetClientOffset(cls, c.Alias)
		cl := curp.NewClient(b, len(c.ReplicaAddrs), c.Reqs, pclients)
		if cl == nil {
			return nil, 0
		}
		// Use HybridLoop with weakRatio=0 to collect metrics (Phase 52.4)
		// CURP only supports strong consistency, so all commands are strong
		hbc := client.NewHybridBufferClient(b, 0, 0) // weakRatio=0, weakWrites=0
		hbc.SetHybridClient(cl)
		printResults := (numThreads == 1)
		hbc.HybridLoopWithOptions(printResults)
		return hbc.GetMetrics(), hbc.GetDuration()
	} else if p == "curpht" {
		cls := []string{}
		for a := range c.ClientAddrs {
			cls = append(cls, a)
		}
		sort.Slice(cls, func(i, j int) bool {
			return cls[i] < cls[j]
		})
		pclients := c.GetClientOffset(cls, c.Alias)
		cl := curpht.NewClient(b, len(c.ReplicaAddrs), c.Reqs, pclients)
		if cl == nil {
			return nil, 0
		}
		// Always use HybridLoop for curpht to get consistent output format
		// HybridLoop handles all-strong workloads (weakRatio=0) correctly
		weakWrites := c.WeakWrites
		if weakWrites == 0 && c.WeakRatio > 0 {
			// Default weakWrites to 50 if weak commands are enabled but no ratio specified
			weakWrites = 50
		}
		hbc := client.NewHybridBufferClient(b, c.WeakRatio, weakWrites)
		hbc.SetHybridClient(cl)
		// For single thread, run with printing. For multiple threads, collect metrics.
		printResults := (numThreads == 1)
		hbc.HybridLoopWithOptions(printResults)
		return hbc.GetMetrics(), hbc.GetDuration()
	} else if p == "curpho" {
		cls := []string{}
		for a := range c.ClientAddrs {
			cls = append(cls, a)
		}
		sort.Slice(cls, func(i, j int) bool {
			return cls[i] < cls[j]
		})
		pclients := c.GetClientOffset(cls, c.Alias)
		cl := curpho.NewClient(b, len(c.ReplicaAddrs), c.Reqs, pclients)
		if cl == nil {
			return nil, 0
		}
		// Always use HybridLoop for curpho to get consistent output format
		weakWrites := c.WeakWrites
		if weakWrites == 0 && c.WeakRatio > 0 {
			weakWrites = 50
		}
		hbc := client.NewHybridBufferClient(b, c.WeakRatio, weakWrites)
		hbc.SetHybridClient(cl)
		printResults := (numThreads == 1)
		hbc.HybridLoopWithOptions(printResults)
		return hbc.GetMetrics(), hbc.GetDuration()
	} else if p == "raft" {
		raftCl := raft.NewClient(b)
		hbc := client.NewHybridBufferClient(b, 0, 0) // weakRatio=0: all strong
		hbc.SetHybridClient(raftCl)
		printResults := (numThreads == 1)
		hbc.HybridLoopWithOptions(printResults)
		return hbc.GetMetrics(), hbc.GetDuration()
	} else if p == "raftht" {
		rafthtCl := raftht.NewClient(b)
		weakWrites := c.WeakWrites
		if weakWrites == 0 && c.WeakRatio > 0 {
			weakWrites = 50
		}
		hbc := client.NewHybridBufferClient(b, c.WeakRatio, weakWrites)
		hbc.SetHybridClient(rafthtCl)
		printResults := (numThreads == 1)
		hbc.HybridLoopWithOptions(printResults)
		return hbc.GetMetrics(), hbc.GetDuration()
	} else {
		waitFrom := b.LeaderId
		if b.Fast || b.Leaderless || c.WaitClosest {
			waitFrom = b.ClosestId
		}
		b.WaitReplies(waitFrom)
		b.Loop()
		return nil, 0
	}
}
