package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	_ "net/http/pprof" // Enable pprof endpoints for profiling
	"net/rpc"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/imdea-software/swiftpaxos/config"
	"github.com/imdea-software/swiftpaxos/curp"
	curpht "github.com/imdea-software/swiftpaxos/curp-ht"
	curpho "github.com/imdea-software/swiftpaxos/curp-ho"
	"github.com/imdea-software/swiftpaxos/dlog"
	"github.com/imdea-software/swiftpaxos/epaxos"
	epaxosho "github.com/imdea-software/swiftpaxos/epaxos-ho"
	epaxosswift "github.com/imdea-software/swiftpaxos/epaxos-swift"
	"github.com/imdea-software/swiftpaxos/fastpaxos"
	"github.com/imdea-software/swiftpaxos/n2paxos"
	"github.com/imdea-software/swiftpaxos/paxos"
	"github.com/imdea-software/swiftpaxos/raft"
	raftht "github.com/imdea-software/swiftpaxos/raft-ht"
	"github.com/imdea-software/swiftpaxos/mongotunable"
	"github.com/imdea-software/swiftpaxos/pileus"
	"github.com/imdea-software/swiftpaxos/pileusht"
	"github.com/imdea-software/swiftpaxos/replica/defs"
	"github.com/imdea-software/swiftpaxos/swift"
)

func runReplica(c *config.Config, logger *dlog.Logger) {
	// Derive port and replica index from alias.
	// e.g., "replica0" → port 7070, index 0; "replica3" → port 7073, index 3
	port := 7070
	aliasIdx := 0
	for i, ch := range c.Alias {
		if ch >= '0' && ch <= '9' {
			idx, err := strconv.Atoi(c.Alias[i:])
			if err == nil {
				port = 7070 + idx
				aliasIdx = idx
			}
			break
		}
	}

	log.Printf("Server starting on port %d", port)
	maddr := fmt.Sprintf("%s:%d", c.MasterAddr, c.MasterPort)
	addr := c.ReplicaAddrs[c.Alias]
	replicaId, nodeList, isLeader := registerWithMaster(addr, maddr, port, aliasIdx)
	f := (len(c.ReplicaAddrs) - 1) / 2
	log.Printf("Tolerating %d max. failures", f)

	switch strings.ToLower(c.Protocol) {
	case "swiftpaxos":
		log.Println("Starting SwiftPaxos replica...")
		if c.MaxDescRoutines > 0 {
			swift.MaxDescRoutines = c.MaxDescRoutines
		}
		rep := swift.New(c.Alias, replicaId, nodeList, !c.Noop,
			c.Optread, true, false, 1, f, c, logger, nil)
		rpc.Register(rep)
	case "curp":
		log.Println("Starting optimized CURP replica...")
		if c.MaxDescRoutines > 0 {
			curp.MaxDescRoutines = c.MaxDescRoutines
		}
		rep := curp.New(c.Alias, replicaId, nodeList, !c.Noop,
			1, f, true, c, logger)
		rpc.Register(rep)
	case "curpht":
		log.Println("Starting CURP-HT (Hybrid Transparency) replica...")
		if c.MaxDescRoutines > 0 {
			curpht.MaxDescRoutines = c.MaxDescRoutines
		}
		rep := curpht.New(c.Alias, replicaId, nodeList, !c.Noop,
			1, f, true, c, logger)
		rpc.Register(rep)
	case "curpho":
		log.Println("Starting CURP-HO (Hybrid Optimal) replica...")
		if c.MaxDescRoutines > 0 {
			curpho.MaxDescRoutines = c.MaxDescRoutines
		}
		rep := curpho.New(c.Alias, replicaId, nodeList, !c.Noop,
			1, f, true, c, logger)
		rpc.Register(rep)
	case "fastpaxos":
		log.Println("Starting Fast Paxos replica...")
		rep := fastpaxos.New(c.Alias, replicaId, nodeList, !c.Noop, f, c, logger)
		rpc.Register(rep)
	case "n2paxos":
		log.Println("Starting N²Paxos replica...")
		rep := n2paxos.New(c.Alias, replicaId, nodeList, !c.Noop, 1, f, c, logger)
		rpc.Register(rep)
	case "paxos":
		log.Println("Starting Paxos replica...")
		rep := paxos.New(c.Alias, replicaId, nodeList, isLeader, f, c, logger)
		rpc.Register(rep)
	case "epaxos":
		log.Println("Starting EPaxos replica...")
		rep := epaxos.New(c.Alias, replicaId, nodeList, !c.Noop, false, false, 0, false, f, c, logger)
		rpc.Register(rep)
	case "epaxosswift":
		log.Println("Starting EPaxos-Swift replica...")
		rep := epaxosswift.New(c.Alias, replicaId, nodeList, !c.Noop, false, false, 0, false, f, c, logger)
		rpc.Register(rep)
	case "epaxosho":
		log.Println("Starting EPaxos-HO replica...")
		rep := epaxosho.New(c.Alias, replicaId, nodeList, !c.Noop, false, false, 0, f, c, logger)
		rpc.Register(rep)
	case "raft":
		log.Println("Starting Raft replica...")
		rep := raft.New(c.Alias, replicaId, nodeList, isLeader, f, c, logger)
		rpc.Register(rep)
	case "raftht":
		log.Println("Starting Raft-HT replica...")
		rep := raftht.New(c.Alias, replicaId, nodeList, isLeader, f, c, logger)
		rpc.Register(rep)
	case "mongotunable":
		log.Println("Starting MongoDB-Tunable replica...")
		rep := mongotunable.New(c.Alias, replicaId, nodeList, isLeader, f, c, logger)
		rpc.Register(rep)
	case "pileus":
		log.Println("Starting Pileus replica...")
		rep := pileus.New(c.Alias, replicaId, nodeList, isLeader, f, c, logger)
		rpc.Register(rep)
	case "pileusht":
		log.Println("Starting Pileus-HT replica...")
		rep := pileusht.New(c.Alias, replicaId, nodeList, isLeader, f, c, logger)
		rpc.Register(rep)
	}

	rpc.HandleHTTP()
	// Bind RPC listener to specific IP to allow multiple replicas on same machine.
	// Use SO_REUSEADDR to avoid TIME_WAIT conflicts between consecutive benchmark runs.
	lc := net.ListenConfig{
		Control: func(network, address string, c syscall.RawConn) error {
			return c.Control(func(fd uintptr) {
				syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
			})
		},
	}
	l, err := lc.Listen(context.Background(), "tcp", fmt.Sprintf("%s:%d", addr, port+1000))
	if err != nil {
		log.Fatal("listen error:", err)
	}
	http.Serve(l, nil)
}

func registerWithMaster(addr, mAddr string, port int, replicaId int) (int, []string, bool) {
	var reply defs.RegisterReply
	args := &defs.RegisterArgs{
		Addr:      addr,
		Port:      port,
		ReplicaId: replicaId,
	}
	log.Printf("connecting to: %v", mAddr)

	for {
		mcli, err := rpc.DialHTTP("tcp", mAddr)
		if err == nil {
			for {
				// TODO: This is an active wait...
				err = mcli.Call("Master.Register", args, &reply)
				if err == nil {
					if reply.Ready {
						break
					}
					time.Sleep(4)
				} else {
					log.Printf("%v", err)
				}
			}
			break
		} else {
			log.Printf("%v", err)
		}
		time.Sleep(4)
	}

	return reply.ReplicaId, reply.NodeList, reply.IsLeader
}
