// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import {IVault} from "./IVault.sol";

/// @title ISettlement - Eidos DEX Settlement Interface
/// @notice Interface for the Settlement contract that processes trade settlements
interface ISettlement {
    // =============================================================
    //                           STRUCTS
    // =============================================================

    /// @notice Represents a single trade to be settled
    struct Trade {
        bytes32 tradeId;        // Unique trade identifier
        address maker;          // Maker address
        address taker;          // Taker address
        address baseToken;      // Base token (e.g., ETH)
        address quoteToken;     // Quote token (e.g., USDC)
        uint256 baseAmount;     // Amount of base token
        uint256 quoteAmount;    // Amount of quote token
        uint256 makerFee;       // Fee paid by maker (in quote token)
        uint256 takerFee;       // Fee paid by taker (in quote token)
        bool isBuy;             // True if maker is buying base token
    }

    /// @notice Represents a batch of trades to be settled
    struct SettlementBatch {
        bytes32 batchId;        // Unique batch identifier
        Trade[] trades;         // Array of trades
        uint256 timestamp;      // Batch creation timestamp
        bytes operatorSig;      // Operator signature
    }

    // =============================================================
    //                           EVENTS
    // =============================================================

    /// @notice Emitted when a batch is settled
    event BatchSettled(bytes32 indexed batchId, uint256 tradeCount, uint256 timestamp);

    /// @notice Emitted when a single trade is settled
    event TradeSettled(
        bytes32 indexed batchId,
        bytes32 indexed tradeId,
        address indexed maker,
        address taker,
        address baseToken,
        address quoteToken,
        uint256 baseAmount,
        uint256 quoteAmount
    );

    /// @notice Emitted when fees are collected
    event FeesCollected(address indexed token, uint256 amount);

    /// @notice Emitted when fee recipient is updated
    event FeeRecipientUpdated(address indexed oldRecipient, address indexed newRecipient);

    /// @notice Emitted when vault is updated
    event VaultUpdated(address indexed oldVault, address indexed newVault);

    /// @notice Emitted when operator is updated
    event OperatorUpdated(address indexed oldOperator, address indexed newOperator);

    // =============================================================
    //                           ERRORS
    // =============================================================

    error InvalidAddress();
    error InvalidBatch();
    error InvalidSignature();
    error BatchAlreadySettled();
    error EmptyBatch();
    error TradeAlreadySettled();
    error Unauthorized();
    error SettlementFailed();
    error InvalidTradeData();

    // =============================================================
    //                    SETTLEMENT FUNCTIONS
    // =============================================================

    /// @notice Settle a batch of trades
    /// @param batch The settlement batch containing trades
    function settleBatch(SettlementBatch calldata batch) external;

    /// @notice Settle a single trade (emergency function)
    /// @param trade The trade to settle
    /// @param operatorSig The operator signature
    function settleSingleTrade(Trade calldata trade, bytes calldata operatorSig) external;

    // =============================================================
    //                        VIEW FUNCTIONS
    // =============================================================

    /// @notice Check if a batch has been settled
    /// @param batchId The batch identifier
    /// @return True if the batch has been settled
    function isBatchSettled(bytes32 batchId) external view returns (bool);

    /// @notice Check if a trade has been settled
    /// @param tradeId The trade identifier
    /// @return True if the trade has been settled
    function isTradeSettled(bytes32 tradeId) external view returns (bool);

    /// @notice Get the vault address
    /// @return The vault contract
    function vault() external view returns (IVault);

    /// @notice Get the operator address
    /// @return The operator address
    function operator() external view returns (address);

    /// @notice Get the fee recipient address
    /// @return The fee recipient address
    function feeRecipient() external view returns (address);

    /// @notice Get total fees collected for a token
    /// @param token The token address
    /// @return The total fees collected
    function totalFeesCollected(address token) external view returns (uint256);
}
