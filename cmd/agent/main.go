package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"agentmesh/pkg/agent"
)

func main() {
	dbPath := flag.String("db", "agent_metadata.db", "Path to metadata database")
	workspace := flag.String("workspace", "./workspace", "Path to OpenClaw workspace")
	listenAddr := flag.String("listen", "/ip4/0.0.0.0/tcp/0", "libp2p listen address")
	rpcURL := flag.String("rpc", "https://sepolia.base.org", "Ethereum RPC URL")
	escrowAddr := flag.String("escrow", "0x591ee5158c94d736ce9bf544bc03247d14904061", "TaskEscrow contract address")
	marketAddr := flag.String("market", "0x051509a30a62b1ea250eef5ad924d0690a4d20e6", "KnowledgeMarket contract address")
	identAddr := flag.String("identity", "0x8004A169FB4a3325136EB29fA0ceB6D2e539a432", "ERC-8004 IdentityRegistry address")

	flag.Parse()

	fmt.Printf("Starting AgentMesh Node...\n")
	fmt.Printf("Database: %s\n", *dbPath)
	fmt.Printf("Workspace: %s\n", *workspace)

	// Ensure workspace exists
	os.MkdirAll(*workspace, 0755)

	node, err := agent.NewAgentNode(*dbPath, *workspace)
	if err != nil {
		log.Fatalf("Failed to initialize node: %v", err)
	}

	// Setup ERC8004 Client (Mock/Placeholder addresses for Reputation/Validation)
	node.ERCClient = agent.NewERC8004Client(*rpcURL, *identAddr, "0x0000000000000000000000000000000000000000", "0x0000000000000000000000000000000000000000")

	// Setup Watcher
	watcher, err := agent.NewEventWatcher(*rpcURL, *escrowAddr, *marketAddr, func(e agent.TaskCreatedEvent) {
		fmt.Printf("[Watcher] New Task Created on-chain: %s\n", e.TaskId)
	}, func(q agent.KnowledgeRequestedEvent) {
		fmt.Printf("[Watcher] New Knowledge Request on-chain: %s (Bounty: %s)\n", q.Topic, q.Bounty)

		// Dynamic Identity Resolution: wallet -> agentId -> peerId
		if node.ERCClient != nil {
			agentId, err := node.ERCClient.GetAgentIdByWallet(q.Requester)
			if err == nil {
				peerId, err := node.ERCClient.GetMetadata(agentId, "peerId")
				if err == nil && peerId != "" {
					fmt.Printf("[Discovery] Resolved PeerID for %s: %s\n", q.Requester.Hex(), peerId)
					// Trigger P2P delivery here...
				}
			}
		}
	})
	if err == nil {
		node.Watcher = watcher
		go node.Watcher.Start(context.Background())
	}

	if err := node.Start(*listenAddr); err != nil {
		log.Fatalf("Failed to start node: %v", err)
	}

	fmt.Printf("Node started! ID: %s\n", node.Host.ID())
	fmt.Printf("Addresses: %v\n", node.Host.Addrs())

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig

	node.Stop()
	fmt.Println("Node stopped.")
}
