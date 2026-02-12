// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

import "@openzeppelin/contracts/utils/ReentrancyGuard.sol";
import "@openzeppelin/contracts/access/Ownable.sol";

/**
 * @title JuryPool
 * @notice Manages agent registration for dispute resolution jury duty
 */
contract JuryPool is ReentrancyGuard, Ownable {
    
    struct Juror {
        address agent;
        uint256 stake;
        uint256 reputation;  // Cached from ERC-8004
        uint256 casesJudged;
        uint256 correctVerdicts;
        bool active;
    }
    
    uint256 public constant MIN_STAKE = 0.01 ether;
    uint256 public constant MIN_REPUTATION = 10;
    
    address[] public jurorList;
    mapping(address => Juror) public jurors;
    mapping(address => uint256) public jurorIndex; // For O(1) lookup
    
    address public disputeResolver;
    address public reputationRegistry; // ERC-8004 contract
    
    event JurorRegistered(address indexed agent, uint256 stake);
    event JurorWithdrawn(address indexed agent, uint256 stake);
    event JurorSelected(address indexed agent, uint256 disputeId);
    event ReputationUpdated(address indexed agent, uint256 newReputation);
    
    constructor() Ownable(msg.sender) {}
    
    function setDisputeResolver(address _resolver) external onlyOwner {
        disputeResolver = _resolver;
    }
    
    function setReputationRegistry(address _registry) external onlyOwner {
        reputationRegistry = _registry;
    }
    
    modifier onlyDisputeResolver() {
        require(msg.sender == disputeResolver, "Not dispute resolver");
        _;
    }
    
    /**
     * @notice Agent registers as a potential juror
     */
    function register() external payable nonReentrant {
        require(msg.value >= MIN_STAKE, "Insufficient stake");
        require(!jurors[msg.sender].active, "Already registered");
        
        // In production, query ERC-8004 for reputation
        uint256 reputation = _getReputation(msg.sender);
        require(reputation >= MIN_REPUTATION, "Reputation too low");
        
        jurors[msg.sender] = Juror({
            agent: msg.sender,
            stake: msg.value,
            reputation: reputation,
            casesJudged: 0,
            correctVerdicts: 0,
            active: true
        });
        
        jurorIndex[msg.sender] = jurorList.length;
        jurorList.push(msg.sender);
        
        emit JurorRegistered(msg.sender, msg.value);
    }
    
    /**
     * @notice Agent withdraws from jury pool
     */
    function withdraw() external nonReentrant {
        Juror storage juror = jurors[msg.sender];
        require(juror.active, "Not registered");
        
        uint256 stake = juror.stake;
        juror.active = false;
        juror.stake = 0;
        
        // Remove from list (swap with last)
        uint256 idx = jurorIndex[msg.sender];
        address lastJuror = jurorList[jurorList.length - 1];
        jurorList[idx] = lastJuror;
        jurorIndex[lastJuror] = idx;
        jurorList.pop();
        
        (bool success, ) = msg.sender.call{value: stake}("");
        require(success, "Withdrawal failed");
        
        emit JurorWithdrawn(msg.sender, stake);
    }
    
    /**
     * @notice Select random jurors for a dispute (weighted by reputation)
     * @param disputeId The dispute requiring jurors
     * @param count Number of jurors to select
     * @param excludeClient Exclude this address
     * @param excludeWorker Exclude this address
     */
    function selectJurors(
        uint256 disputeId,
        uint256 count,
        address excludeClient,
        address excludeWorker
    ) external onlyDisputeResolver returns (address[] memory) {
        require(jurorList.length >= count, "Not enough jurors");
        
        address[] memory selected = new address[](count);
        uint256 selectedCount = 0;
        
        // Simple random selection (in production, use Chainlink VRF or commit-reveal)
        uint256 seed = uint256(keccak256(abi.encodePacked(
            block.timestamp,
            block.prevrandao,
            disputeId
        )));
        
        uint256 attempts = 0;
        uint256 maxAttempts = jurorList.length * 3;
        
        while (selectedCount < count && attempts < maxAttempts) {
            uint256 idx = seed % jurorList.length;
            address candidate = jurorList[idx];
            
            // Skip if already selected, or is a party to the dispute
            bool valid = true;
            if (candidate == excludeClient || candidate == excludeWorker) {
                valid = false;
            }
            for (uint256 i = 0; i < selectedCount; i++) {
                if (selected[i] == candidate) {
                    valid = false;
                    break;
                }
            }
            
            if (valid && jurors[candidate].active) {
                selected[selectedCount] = candidate;
                selectedCount++;
                emit JurorSelected(candidate, disputeId);
            }
            
            seed = uint256(keccak256(abi.encodePacked(seed, attempts)));
            attempts++;
        }
        
        require(selectedCount == count, "Could not select enough jurors");
        return selected;
    }
    
    /**
     * @notice Update juror stats after a verdict
     */
    function recordVerdict(address agent, bool wasCorrect) external onlyDisputeResolver {
        Juror storage juror = jurors[agent];
        if (!juror.active) return;
        
        juror.casesJudged++;
        if (wasCorrect) {
            juror.correctVerdicts++;
        }
    }
    
    /**
     * @notice Slash a juror's stake (for incorrect verdicts)
     */
    function slashStake(address agent, uint256 amount) external onlyDisputeResolver {
        Juror storage juror = jurors[agent];
        require(juror.active, "Not active juror");
        
        if (amount > juror.stake) {
            amount = juror.stake;
        }
        juror.stake -= amount;
        
        // If stake falls below minimum, deactivate
        if (juror.stake < MIN_STAKE) {
            juror.active = false;
        }
    }
    
    /**
     * @notice Get reputation from ERC-8004 (placeholder)
     */
    function _getReputation(address agent) internal view returns (uint256) {
        // In production, call ERC-8004 contract
        // For now, return a default value
        if (reputationRegistry == address(0)) {
            return 100; // Default for testing
        }
        // IReputationRegistry(reputationRegistry).getReputation(agent);
        return 100;
    }
    
    function getActiveJurorCount() external view returns (uint256) {
        return jurorList.length;
    }
    
    function getJuror(address agent) external view returns (Juror memory) {
        return jurors[agent];
    }
}
