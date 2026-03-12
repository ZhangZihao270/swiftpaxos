package master

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/rpc"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/imdea-software/swiftpaxos/dlog"
	"github.com/imdea-software/swiftpaxos/replica/defs"
)

type Master struct {
	*dlog.Logger

	N            int
	port         int
	nodeList     []string
	addrList     []string
	portList     []int
	registered   []bool // tracks which replica IDs have registered
	numRegistered int
	lock         *sync.Mutex
	nodes        []*rpc.Client
	leader       []bool
	alive        []bool
	latencies    []float64
	finishInit   bool
	initCond     *sync.Cond
	nextLeader   int
}

func New(N, port int, logger *dlog.Logger) *Master {
	master := &Master{
		Logger: logger,

		N:             N,
		port:          port,
		nodeList:      make([]string, N),
		addrList:      make([]string, N),
		portList:      make([]int, N),
		registered:    make([]bool, N),
		numRegistered: 0,
		lock:          new(sync.Mutex),
		nodes:         make([]*rpc.Client, N),
		leader:        make([]bool, N),
		alive:         make([]bool, N),
		latencies:     make([]float64, N),
		finishInit:    false,
		nextLeader:    -1,
	}
	master.initCond = sync.NewCond(master.lock)
	return master
}

func (master *Master) Run() {
	master.Printf("master starting on port %d", master.port)
	master.Printf("waiting for %d replicas", master.N)

	rpc.Register(master)
	rpc.HandleHTTP()
	lc := net.ListenConfig{
		Control: func(network, address string, c syscall.RawConn) error {
			return c.Control(func(fd uintptr) {
				syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
			})
		},
	}
	l, err := lc.Listen(context.Background(), "tcp", fmt.Sprintf(":%d", master.port))
	if err != nil {
		master.Fatal("master listen error:", err)
	}
	go master.run()
	http.Serve(l, nil)
}

func (master *Master) run() {
	for {
		master.lock.Lock()
		if master.numRegistered == master.N {
			master.lock.Unlock()
			break
		}
		master.lock.Unlock()
		time.Sleep(time.Second)
	}
	time.Sleep(2 * time.Second)

	for i := 0; i < master.N; {
		var err error
		addr := fmt.Sprintf("%s:%d", master.addrList[i], master.portList[i]+1000)
		master.nodes[i], err = rpc.DialHTTP("tcp", addr)
		if err != nil {
			master.Printf("error connecting to replica %d (%v), retrying...", i, addr)
			time.Sleep(time.Second)
		} else {
			btlReply := defs.NewBeTheLeaderReply()
			if master.leader[i] {
				err = master.nodes[i].Call("Replica.BeTheLeader", &defs.BeTheLeaderArgs{}, btlReply)
				if err != nil {
					master.Fatal("Not today Zurg!")
				}
				defs.UpdateBeTheLeaderReply(btlReply)
				if btlReply.Leader != -1 && btlReply.Leader != int32(i) {
					master.leader[i] = false
					master.leader[int(btlReply.Leader)] = true
				}
				master.nextLeader = int(btlReply.NextLeader)
			}
			i++
		}
	}

	var new_leader bool
	pingNode := func(i int, node *rpc.Client) {
		err := node.Call("Replica.Ping", &defs.PingArgs{}, &defs.PingReply{})
		if err != nil {
			master.alive[i] = false
			if master.leader[i] {
				new_leader = true
				master.leader[i] = false
			}
		} else {
			master.alive[i] = true
		}
	}
	master.lock.Lock()
	for i, node := range master.nodes {
		pingNode(i, node)
	}
	// initialization is finished
	// (i.e., `alive` has been computed)
	master.finishInit = true
	master.initCond.Broadcast()
	master.lock.Unlock()

	beTheLeader := func(i int) error {
		if master.alive[i] {
			btlReply := defs.NewBeTheLeaderReply()
			err := master.nodes[i].Call("Replica.BeTheLeader", &defs.BeTheLeaderArgs{}, btlReply)
			if err == nil {
				defs.UpdateBeTheLeaderReply(btlReply)
				leaderI := i
				if btlReply.Leader != -1 {
					leaderI = int(btlReply.Leader)
				}
				master.leader[leaderI] = true
				master.nextLeader = int(btlReply.NextLeader)
				master.Printf("replica %d is the new leader", leaderI)
				return nil
			}
			return err
		}
		return errors.New("dead")
	}

	for {
		time.Sleep(3 * time.Second)
		new_leader = false
		for i, node := range master.nodes {
			pingNode(i, node)
		}

		if !new_leader {
			continue
		}
		if master.nextLeader != -1 {
			if beTheLeader(master.nextLeader) == nil {
				continue
			}
		}
		for i := range master.nodes {
			if beTheLeader(i) == nil {
				break
			}
		}
	}
}

func (master *Master) Register(args *defs.RegisterArgs, reply *defs.RegisterReply) error {
	master.lock.Lock()
	defer master.lock.Unlock()

	index := args.ReplicaId
	if index < 0 || index >= master.N {
		return fmt.Errorf("invalid ReplicaId %d (N=%d)", index, master.N)
	}

	addrPort := fmt.Sprintf("%s:%d", args.Addr, args.Port)

	if !master.registered[index] {
		master.nodeList[index] = addrPort
		master.addrList[index] = args.Addr
		master.portList[index] = args.Port
		master.registered[index] = true
		master.leader[index] = false
		master.numRegistered++

		addr := args.Addr
		if addr == "" {
			addr = "127.0.0.1"
		}
		out, err := exec.Command("ping", addr, "-c 2", "-q").Output()
		if err == nil {
			master.latencies[index], _ =
				strconv.ParseFloat(strings.Split(string(out), "/")[4], 64)
			master.Printf("node %v [%v] -> %v", index,
				master.nodeList[index], master.latencies[index])
		} else {
			master.Fatal("cannot connect to " + addr)
		}
	}

	if master.numRegistered == master.N {
		reply.Ready = true
		reply.ReplicaId = index
		reply.NodeList = master.nodeList

		// Always use replica 0 as the initial leader.
		// Set leader[0] once when all replicas are registered.
		if !master.leader[0] {
			master.leader[0] = true
			master.Printf("replica 0 is the new leader")
		}
		reply.IsLeader = (index == 0)
	} else {
		reply.Ready = false
	}

	return nil
}

func (master *Master) GetLeader(args *defs.GetLeaderArgs, reply *defs.GetLeaderReply) error {
	master.lock.Lock()
	defer master.lock.Unlock()

	for i, l := range master.leader {
		if l {
			*reply = defs.GetLeaderReply{
				LeaderId: i,
			}
			break
		}
	}
	return nil
}

func (master *Master) GetReplicaList(args *defs.GetReplicaListArgs, reply *defs.GetReplicaListReply) error {
	master.lock.Lock()

	for !master.finishInit {
		master.initCond.Wait()
	}

	if len(master.nodeList) == master.N {
		reply.Ready = true
	} else {
		reply.Ready = false
	}

	reply.ReplicaList = make([]string, 0)
	reply.AliveList = make([]bool, 0)
	for i, node := range master.nodeList {
		reply.ReplicaList = append(reply.ReplicaList, node)
		reply.AliveList = append(reply.AliveList, master.alive[i])
	}

	master.Printf("nodes list %v", reply.ReplicaList)
	master.lock.Unlock()
	return nil
}
