// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

/// @title IVault - Eidos DEX Vault Interface
/// @notice Interface for the Vault contract that holds user funds
interface IVault {
    // =============================================================
    //                           EVENTS
    // =============================================================

    /// @notice Emitted when a user deposits tokens
    event Deposit(address indexed user, address indexed token, uint256 amount);

    /// @notice Emitted when a user withdraws tokens
    event Withdraw(address indexed user, address indexed token, uint256 amount);

    /// @notice Emitted when funds are frozen for an order
    event FundsLocked(address indexed user, address indexed token, uint256 amount, bytes32 indexed orderId);

    /// @notice Emitted when frozen funds are unlocked (order cancelled)
    event FundsUnlocked(address indexed user, address indexed token, uint256 amount, bytes32 indexed orderId);

    /// @notice Emitted when a settlement transfers funds
    event SettlementTransfer(
        address indexed from,
        address indexed to,
        address indexed token,
        uint256 amount,
        bytes32 settlementId
    );

    /// @notice Emitted when operator is updated
    event OperatorUpdated(address indexed oldOperator, address indexed newOperator);

    /// @notice Emitted when settlement contract is updated
    event SettlementContractUpdated(address indexed oldContract, address indexed newContract);

    // =============================================================
    //                           ERRORS
    // =============================================================

    error InvalidAddress();
    error InvalidAmount();
    error InsufficientBalance();
    error InsufficientLockedBalance();
    error Unauthorized();
    error TokenNotSupported();
    error DepositFailed();
    error WithdrawFailed();
    error TransferFailed();

    // =============================================================
    //                        USER FUNCTIONS
    // =============================================================

    /// @notice Deposit ERC20 tokens into the vault
    /// @param token The token address to deposit
    /// @param amount The amount to deposit
    function deposit(address token, uint256 amount) external;

    /// @notice Deposit ETH into the vault
    function depositETH() external payable;

    /// @notice Withdraw tokens from the vault
    /// @param token The token address to withdraw
    /// @param amount The amount to withdraw
    function withdraw(address token, uint256 amount) external;

    /// @notice Withdraw ETH from the vault
    /// @param amount The amount of ETH to withdraw
    function withdrawETH(uint256 amount) external;

    // =============================================================
    //                      OPERATOR FUNCTIONS
    // =============================================================

    /// @notice Lock user funds for an order (called by operator)
    /// @param user The user address
    /// @param token The token address
    /// @param amount The amount to lock
    /// @param orderId The order ID
    function lockFunds(address user, address token, uint256 amount, bytes32 orderId) external;

    /// @notice Unlock user funds when order is cancelled (called by operator)
    /// @param user The user address
    /// @param token The token address
    /// @param amount The amount to unlock
    /// @param orderId The order ID
    function unlockFunds(address user, address token, uint256 amount, bytes32 orderId) external;

    // =============================================================
    //                    SETTLEMENT FUNCTIONS
    // =============================================================

    /// @notice Transfer locked funds during settlement (called by settlement contract)
    /// @param from The sender address
    /// @param to The receiver address
    /// @param token The token address
    /// @param amount The amount to transfer
    /// @param settlementId The settlement batch ID
    function settlementTransfer(
        address from,
        address to,
        address token,
        uint256 amount,
        bytes32 settlementId
    ) external;

    // =============================================================
    //                        VIEW FUNCTIONS
    // =============================================================

    /// @notice Get user's available balance (total - locked)
    /// @param user The user address
    /// @param token The token address
    /// @return The available balance
    function getAvailableBalance(address user, address token) external view returns (uint256);

    /// @notice Get user's total balance
    /// @param user The user address
    /// @param token The token address
    /// @return The total balance
    function getTotalBalance(address user, address token) external view returns (uint256);

    /// @notice Get user's locked balance
    /// @param user The user address
    /// @param token The token address
    /// @return The locked balance
    function getLockedBalance(address user, address token) external view returns (uint256);

    /// @notice Check if a token is supported
    /// @param token The token address
    /// @return True if the token is supported
    function isTokenSupported(address token) external view returns (bool);
}
