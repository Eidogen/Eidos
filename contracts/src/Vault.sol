// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import {IVault} from "./interfaces/IVault.sol";
import {IERC20} from "./interfaces/IERC20.sol";
import {ReentrancyGuard} from "./utils/ReentrancyGuard.sol";
import {Ownable} from "./utils/Ownable.sol";

/// @title Vault - Eidos DEX Fund Custody Contract
/// @notice Holds user funds for off-chain trading with on-chain settlement
/// @dev Supports ETH and ERC20 tokens. Uses locked balances for order management.
contract Vault is IVault, ReentrancyGuard, Ownable {
    // =============================================================
    //                          CONSTANTS
    // =============================================================

    /// @notice Native ETH address representation
    address public constant ETH = address(0);

    // =============================================================
    //                          STORAGE
    // =============================================================

    /// @notice Operator address authorized to lock/unlock funds
    address public operator;

    /// @notice Settlement contract address
    address public settlementContract;

    /// @notice User balances: user => token => balance
    mapping(address => mapping(address => uint256)) private _balances;

    /// @notice User locked balances: user => token => locked amount
    mapping(address => mapping(address => uint256)) private _lockedBalances;

    /// @notice Supported tokens
    mapping(address => bool) private _supportedTokens;

    /// @notice Token whitelist enabled flag
    bool public tokenWhitelistEnabled;

    // =============================================================
    //                         MODIFIERS
    // =============================================================

    modifier onlyOperator() {
        if (msg.sender != operator) revert Unauthorized();
        _;
    }

    modifier onlySettlement() {
        if (msg.sender != settlementContract) revert Unauthorized();
        _;
    }

    modifier validAddress(address addr) {
        if (addr == address(0)) revert InvalidAddress();
        _;
    }

    modifier validAmount(uint256 amount) {
        if (amount == 0) revert InvalidAmount();
        _;
    }

    modifier tokenSupported(address token) {
        if (tokenWhitelistEnabled && !_supportedTokens[token]) {
            revert TokenNotSupported();
        }
        _;
    }

    // =============================================================
    //                        CONSTRUCTOR
    // =============================================================

    /// @notice Initialize the Vault contract
    /// @param _owner The owner address
    /// @param _operator The operator address
    constructor(address _owner, address _operator) Ownable(_owner) {
        if (_operator == address(0)) revert InvalidAddress();
        operator = _operator;

        // ETH is always supported
        _supportedTokens[ETH] = true;
        tokenWhitelistEnabled = false;
    }

    // =============================================================
    //                      RECEIVE FUNCTION
    // =============================================================

    /// @notice Receive ETH directly
    receive() external payable {
        _balances[msg.sender][ETH] += msg.value;
        emit Deposit(msg.sender, ETH, msg.value);
    }

    // =============================================================
    //                      USER FUNCTIONS
    // =============================================================

    /// @inheritdoc IVault
    function deposit(
        address token,
        uint256 amount
    ) external nonReentrant validAddress(token) validAmount(amount) tokenSupported(token) {
        if (token == ETH) revert InvalidAddress();

        // Transfer tokens from user to vault
        bool success = IERC20(token).transferFrom(msg.sender, address(this), amount);
        if (!success) revert DepositFailed();

        _balances[msg.sender][token] += amount;

        emit Deposit(msg.sender, token, amount);
    }

    /// @inheritdoc IVault
    function depositETH() external payable nonReentrant validAmount(msg.value) {
        _balances[msg.sender][ETH] += msg.value;
        emit Deposit(msg.sender, ETH, msg.value);
    }

    /// @inheritdoc IVault
    function withdraw(
        address token,
        uint256 amount
    ) external nonReentrant validAddress(token) validAmount(amount) {
        if (token == ETH) revert InvalidAddress();

        uint256 available = _getAvailableBalance(msg.sender, token);
        if (available < amount) revert InsufficientBalance();

        _balances[msg.sender][token] -= amount;

        bool success = IERC20(token).transfer(msg.sender, amount);
        if (!success) revert WithdrawFailed();

        emit Withdraw(msg.sender, token, amount);
    }

    /// @inheritdoc IVault
    function withdrawETH(uint256 amount) external nonReentrant validAmount(amount) {
        uint256 available = _getAvailableBalance(msg.sender, ETH);
        if (available < amount) revert InsufficientBalance();

        _balances[msg.sender][ETH] -= amount;

        (bool success, ) = msg.sender.call{value: amount}("");
        if (!success) revert WithdrawFailed();

        emit Withdraw(msg.sender, ETH, amount);
    }

    // =============================================================
    //                     OPERATOR FUNCTIONS
    // =============================================================

    /// @inheritdoc IVault
    function lockFunds(
        address user,
        address token,
        uint256 amount,
        bytes32 orderId
    ) external onlyOperator validAddress(user) validAmount(amount) {
        uint256 available = _getAvailableBalance(user, token);
        if (available < amount) revert InsufficientBalance();

        _lockedBalances[user][token] += amount;

        emit FundsLocked(user, token, amount, orderId);
    }

    /// @inheritdoc IVault
    function unlockFunds(
        address user,
        address token,
        uint256 amount,
        bytes32 orderId
    ) external onlyOperator validAddress(user) validAmount(amount) {
        if (_lockedBalances[user][token] < amount) {
            revert InsufficientLockedBalance();
        }

        _lockedBalances[user][token] -= amount;

        emit FundsUnlocked(user, token, amount, orderId);
    }

    // =============================================================
    //                   SETTLEMENT FUNCTIONS
    // =============================================================

    /// @inheritdoc IVault
    function settlementTransfer(
        address from,
        address to,
        address token,
        uint256 amount,
        bytes32 settlementId
    ) external onlySettlement validAddress(from) validAddress(to) validAmount(amount) {
        // Check locked balance
        if (_lockedBalances[from][token] < amount) {
            revert InsufficientLockedBalance();
        }

        // Deduct from sender's locked balance
        _lockedBalances[from][token] -= amount;
        _balances[from][token] -= amount;

        // Add to receiver's available balance
        _balances[to][token] += amount;

        emit SettlementTransfer(from, to, token, amount, settlementId);
    }

    // =============================================================
    //                       VIEW FUNCTIONS
    // =============================================================

    /// @inheritdoc IVault
    function getAvailableBalance(address user, address token) external view returns (uint256) {
        return _getAvailableBalance(user, token);
    }

    /// @inheritdoc IVault
    function getTotalBalance(address user, address token) external view returns (uint256) {
        return _balances[user][token];
    }

    /// @inheritdoc IVault
    function getLockedBalance(address user, address token) external view returns (uint256) {
        return _lockedBalances[user][token];
    }

    /// @inheritdoc IVault
    function isTokenSupported(address token) external view returns (bool) {
        if (!tokenWhitelistEnabled) return true;
        return _supportedTokens[token];
    }

    // =============================================================
    //                      ADMIN FUNCTIONS
    // =============================================================

    /// @notice Set the operator address
    /// @param newOperator The new operator address
    function setOperator(address newOperator) external onlyOwner validAddress(newOperator) {
        address oldOperator = operator;
        operator = newOperator;
        emit OperatorUpdated(oldOperator, newOperator);
    }

    /// @notice Set the settlement contract address
    /// @param newSettlement The new settlement contract address
    function setSettlementContract(address newSettlement) external onlyOwner validAddress(newSettlement) {
        address oldSettlement = settlementContract;
        settlementContract = newSettlement;
        emit SettlementContractUpdated(oldSettlement, newSettlement);
    }

    /// @notice Add a supported token
    /// @param token The token address to add
    function addSupportedToken(address token) external onlyOwner {
        _supportedTokens[token] = true;
    }

    /// @notice Remove a supported token
    /// @param token The token address to remove
    function removeSupportedToken(address token) external onlyOwner {
        if (token == ETH) revert InvalidAddress();
        _supportedTokens[token] = false;
    }

    /// @notice Enable or disable token whitelist
    /// @param enabled Whether to enable the whitelist
    function setTokenWhitelistEnabled(bool enabled) external onlyOwner {
        tokenWhitelistEnabled = enabled;
    }

    /// @notice Emergency withdraw by owner (only for stuck tokens)
    /// @param token The token address
    /// @param amount The amount to withdraw
    /// @param to The recipient address
    function emergencyWithdraw(
        address token,
        uint256 amount,
        address to
    ) external onlyOwner validAddress(to) validAmount(amount) {
        if (token == ETH) {
            (bool success, ) = to.call{value: amount}("");
            if (!success) revert WithdrawFailed();
        } else {
            bool success = IERC20(token).transfer(to, amount);
            if (!success) revert WithdrawFailed();
        }
    }

    // =============================================================
    //                    INTERNAL FUNCTIONS
    // =============================================================

    /// @notice Get user's available balance (total - locked)
    function _getAvailableBalance(address user, address token) internal view returns (uint256) {
        uint256 total = _balances[user][token];
        uint256 locked = _lockedBalances[user][token];
        return total > locked ? total - locked : 0;
    }
}
