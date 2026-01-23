// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import {ISettlement} from "./interfaces/ISettlement.sol";
import {IVault} from "./interfaces/IVault.sol";
import {ReentrancyGuard} from "./utils/ReentrancyGuard.sol";
import {Ownable} from "./utils/Ownable.sol";
import {SignatureVerifier} from "./utils/SignatureVerifier.sol";

/// @title Settlement - Eidos DEX Trade Settlement Contract
/// @notice Processes batched trade settlements from off-chain matching
/// @dev Uses EIP-712 typed data for signature verification
contract Settlement is ISettlement, ReentrancyGuard, Ownable {
    using SignatureVerifier for bytes32;

    // =============================================================
    //                          CONSTANTS
    // =============================================================

    /// @notice EIP-712 domain separator type hash
    bytes32 public constant DOMAIN_TYPEHASH =
        keccak256("EIP712Domain(string name,string version,uint256 chainId,address verifyingContract)");

    /// @notice Trade struct type hash for EIP-712
    bytes32 public constant TRADE_TYPEHASH =
        keccak256(
            "Trade(bytes32 tradeId,address maker,address taker,address baseToken,address quoteToken,uint256 baseAmount,uint256 quoteAmount,uint256 makerFee,uint256 takerFee,bool isBuy)"
        );

    /// @notice Settlement batch type hash for EIP-712
    bytes32 public constant BATCH_TYPEHASH = keccak256("SettlementBatch(bytes32 batchId,bytes32 tradesHash,uint256 timestamp)");

    // =============================================================
    //                          STORAGE
    // =============================================================

    /// @notice Vault contract reference
    IVault public override vault;

    /// @notice Operator address authorized to sign settlements
    address public override operator;

    /// @notice Fee recipient address
    address public override feeRecipient;

    /// @notice EIP-712 domain separator
    bytes32 public immutable DOMAIN_SEPARATOR;

    /// @notice Settled batches: batchId => settled
    mapping(bytes32 => bool) private _settledBatches;

    /// @notice Settled trades: tradeId => settled
    mapping(bytes32 => bool) private _settledTrades;

    /// @notice Total fees collected per token
    mapping(address => uint256) public override totalFeesCollected;

    // =============================================================
    //                         MODIFIERS
    // =============================================================

    modifier validAddress(address addr) {
        if (addr == address(0)) revert InvalidAddress();
        _;
    }

    // =============================================================
    //                        CONSTRUCTOR
    // =============================================================

    /// @notice Initialize the Settlement contract
    /// @param _owner The owner address
    /// @param _vault The vault contract address
    /// @param _operator The operator address
    /// @param _feeRecipient The fee recipient address
    constructor(
        address _owner,
        address _vault,
        address _operator,
        address _feeRecipient
    ) Ownable(_owner) {
        if (_vault == address(0) || _operator == address(0) || _feeRecipient == address(0)) {
            revert InvalidAddress();
        }

        vault = IVault(_vault);
        operator = _operator;
        feeRecipient = _feeRecipient;

        DOMAIN_SEPARATOR = keccak256(
            abi.encode(
                DOMAIN_TYPEHASH,
                keccak256("EidosSettlement"),
                keccak256("1"),
                block.chainid,
                address(this)
            )
        );
    }

    // =============================================================
    //                   SETTLEMENT FUNCTIONS
    // =============================================================

    /// @inheritdoc ISettlement
    function settleBatch(SettlementBatch calldata batch) external nonReentrant {
        // Validate batch
        if (batch.trades.length == 0) revert EmptyBatch();
        if (_settledBatches[batch.batchId]) revert BatchAlreadySettled();

        // Verify operator signature
        bytes32 batchHash = _hashBatch(batch);
        bytes32 digest = SignatureVerifier.toTypedDataHash(DOMAIN_SEPARATOR, batchHash);
        address signer = digest.recover(batch.operatorSig);
        if (signer != operator) revert InvalidSignature();

        // Mark batch as settled
        _settledBatches[batch.batchId] = true;

        // Process each trade
        for (uint256 i = 0; i < batch.trades.length; i++) {
            _settleTrade(batch.trades[i], batch.batchId);
        }

        emit BatchSettled(batch.batchId, batch.trades.length, batch.timestamp);
    }

    /// @inheritdoc ISettlement
    function settleSingleTrade(Trade calldata trade, bytes calldata operatorSig) external nonReentrant {
        if (_settledTrades[trade.tradeId]) revert TradeAlreadySettled();

        // Verify operator signature
        bytes32 tradeHash = _hashTrade(trade);
        bytes32 digest = SignatureVerifier.toTypedDataHash(DOMAIN_SEPARATOR, tradeHash);
        address signer = digest.recover(operatorSig);
        if (signer != operator) revert InvalidSignature();

        // Create a pseudo batch ID for single trade settlement
        bytes32 batchId = keccak256(abi.encodePacked("single:", trade.tradeId));

        _settleTrade(trade, batchId);

        emit BatchSettled(batchId, 1, block.timestamp);
    }

    // =============================================================
    //                       VIEW FUNCTIONS
    // =============================================================

    /// @inheritdoc ISettlement
    function isBatchSettled(bytes32 batchId) external view returns (bool) {
        return _settledBatches[batchId];
    }

    /// @inheritdoc ISettlement
    function isTradeSettled(bytes32 tradeId) external view returns (bool) {
        return _settledTrades[tradeId];
    }

    // =============================================================
    //                      ADMIN FUNCTIONS
    // =============================================================

    /// @notice Set the vault contract
    /// @param newVault The new vault address
    function setVault(address newVault) external onlyOwner validAddress(newVault) {
        address oldVault = address(vault);
        vault = IVault(newVault);
        emit VaultUpdated(oldVault, newVault);
    }

    /// @notice Set the operator address
    /// @param newOperator The new operator address
    function setOperator(address newOperator) external onlyOwner validAddress(newOperator) {
        address oldOperator = operator;
        operator = newOperator;
        emit OperatorUpdated(oldOperator, newOperator);
    }

    /// @notice Set the fee recipient address
    /// @param newFeeRecipient The new fee recipient address
    function setFeeRecipient(address newFeeRecipient) external onlyOwner validAddress(newFeeRecipient) {
        address oldRecipient = feeRecipient;
        feeRecipient = newFeeRecipient;
        emit FeeRecipientUpdated(oldRecipient, newFeeRecipient);
    }

    // =============================================================
    //                    INTERNAL FUNCTIONS
    // =============================================================

    /// @notice Settle a single trade
    /// @param trade The trade to settle
    /// @param batchId The batch ID
    function _settleTrade(Trade calldata trade, bytes32 batchId) internal {
        // Validate trade
        if (trade.maker == address(0) || trade.taker == address(0)) {
            revert InvalidTradeData();
        }
        if (trade.baseToken == trade.quoteToken) {
            revert InvalidTradeData();
        }
        if (trade.baseAmount == 0 || trade.quoteAmount == 0) {
            revert InvalidTradeData();
        }
        if (_settledTrades[trade.tradeId]) {
            revert TradeAlreadySettled();
        }

        // Mark trade as settled
        _settledTrades[trade.tradeId] = true;

        // Execute transfers based on trade direction
        // isBuy = true means maker is buying base token (selling quote token)
        // isBuy = false means maker is selling base token (buying quote token)

        if (trade.isBuy) {
            // Maker buys base token: maker sends quote, receives base
            // Taker sells base token: taker sends base, receives quote

            // Maker sends quote token to taker
            vault.settlementTransfer(
                trade.maker,
                trade.taker,
                trade.quoteToken,
                trade.quoteAmount,
                batchId
            );

            // Taker sends base token to maker
            vault.settlementTransfer(
                trade.taker,
                trade.maker,
                trade.baseToken,
                trade.baseAmount,
                batchId
            );

            // Collect fees (in quote token)
            if (trade.makerFee > 0) {
                vault.settlementTransfer(
                    trade.maker,
                    feeRecipient,
                    trade.quoteToken,
                    trade.makerFee,
                    batchId
                );
                totalFeesCollected[trade.quoteToken] += trade.makerFee;
            }
            if (trade.takerFee > 0) {
                vault.settlementTransfer(
                    trade.taker,
                    feeRecipient,
                    trade.quoteToken,
                    trade.takerFee,
                    batchId
                );
                totalFeesCollected[trade.quoteToken] += trade.takerFee;
            }
        } else {
            // Maker sells base token: maker sends base, receives quote
            // Taker buys base token: taker sends quote, receives base

            // Maker sends base token to taker
            vault.settlementTransfer(
                trade.maker,
                trade.taker,
                trade.baseToken,
                trade.baseAmount,
                batchId
            );

            // Taker sends quote token to maker
            vault.settlementTransfer(
                trade.taker,
                trade.maker,
                trade.quoteToken,
                trade.quoteAmount,
                batchId
            );

            // Collect fees (in quote token)
            if (trade.makerFee > 0) {
                vault.settlementTransfer(
                    trade.maker,
                    feeRecipient,
                    trade.quoteToken,
                    trade.makerFee,
                    batchId
                );
                totalFeesCollected[trade.quoteToken] += trade.makerFee;
            }
            if (trade.takerFee > 0) {
                vault.settlementTransfer(
                    trade.taker,
                    feeRecipient,
                    trade.quoteToken,
                    trade.takerFee,
                    batchId
                );
                totalFeesCollected[trade.quoteToken] += trade.takerFee;
            }
        }

        emit TradeSettled(
            batchId,
            trade.tradeId,
            trade.maker,
            trade.taker,
            trade.baseToken,
            trade.quoteToken,
            trade.baseAmount,
            trade.quoteAmount
        );
    }

    /// @notice Hash a trade for signature verification
    /// @param trade The trade to hash
    /// @return The trade hash
    function _hashTrade(Trade calldata trade) internal pure returns (bytes32) {
        return keccak256(
            abi.encode(
                TRADE_TYPEHASH,
                trade.tradeId,
                trade.maker,
                trade.taker,
                trade.baseToken,
                trade.quoteToken,
                trade.baseAmount,
                trade.quoteAmount,
                trade.makerFee,
                trade.takerFee,
                trade.isBuy
            )
        );
    }

    /// @notice Hash a batch for signature verification
    /// @param batch The batch to hash
    /// @return The batch hash
    function _hashBatch(SettlementBatch calldata batch) internal pure returns (bytes32) {
        // Hash all trades
        bytes32[] memory tradeHashes = new bytes32[](batch.trades.length);
        for (uint256 i = 0; i < batch.trades.length; i++) {
            tradeHashes[i] = _hashTrade(batch.trades[i]);
        }
        bytes32 tradesHash = keccak256(abi.encodePacked(tradeHashes));

        return keccak256(
            abi.encode(
                BATCH_TYPEHASH,
                batch.batchId,
                tradesHash,
                batch.timestamp
            )
        );
    }
}
