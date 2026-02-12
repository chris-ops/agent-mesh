// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

import "forge-std/Script.sol";
import "../src/TaskEscrow.sol";
import "../src/JuryPool.sol";
import "../src/KnowledgeMarket.sol";

contract DeployScript is Script {
    function run() external {
        vm.startBroadcast();

        // 1. Deploy KnowledgeMarket
        KnowledgeMarket market = new KnowledgeMarket();
        console.log("KnowledgeMarket deployed at:", address(market));

        // 2. Deploy JuryPool
        JuryPool jury = new JuryPool();
        console.log("JuryPool deployed at:", address(jury));

        // 3. Deploy TaskEscrow
        TaskEscrow escrow = new TaskEscrow();
        console.log("TaskEscrow deployed at:", address(escrow));

        // 4. Configuration
        escrow.setJuryPool(address(jury));
        jury.setReputationRegistry(address(0)); // Placeholder for now

        vm.stopBroadcast();
    }
}
