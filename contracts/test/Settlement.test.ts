import { expect } from "chai";
import { ethers } from "hardhat";
import { SignerWithAddress } from "@nomicfoundation/hardhat-ethers/signers";
import { Settlement, Vault, MockERC20 } from "../typechain-types";

describe("Settlement", function () {
  let vault: Vault;
  let settlement: Settlement;
  let baseToken: MockERC20;
  let quoteToken: MockERC20;
  let owner: SignerWithAddress;
  let operator: SignerWithAddress;
  let feeRecipient: SignerWithAddress;
  let maker: SignerWithAddress;
  let taker: SignerWithAddress;

  const INITIAL_BALANCE = ethers.parseEther("10000");
  const DOMAIN_NAME = "EidosSettlement";
  const DOMAIN_VERSION = "1";

  // EIP-712 type hashes
  const TRADE_TYPEHASH = ethers.keccak256(
    ethers.toUtf8Bytes(
      "Trade(bytes32 tradeId,address maker,address taker,address baseToken,address quoteToken,uint256 baseAmount,uint256 quoteAmount,uint256 makerFee,uint256 takerFee,bool isBuy)"
    )
  );

  const BATCH_TYPEHASH = ethers.keccak256(
    ethers.toUtf8Bytes("SettlementBatch(bytes32 batchId,bytes32 tradesHash,uint256 timestamp)")
  );

  beforeEach(async function () {
    [owner, operator, feeRecipient, maker, taker] = await ethers.getSigners();

    // Deploy MockERC20 tokens
    const MockERC20 = await ethers.getContractFactory("MockERC20");
    baseToken = await MockERC20.deploy("Base Token", "BASE", 18);
    quoteToken = await MockERC20.deploy("Quote Token", "QUOTE", 18);
    await baseToken.waitForDeployment();
    await quoteToken.waitForDeployment();

    // Deploy Vault
    const Vault = await ethers.getContractFactory("Vault");
    vault = await Vault.deploy(owner.address, operator.address);
    await vault.waitForDeployment();

    // Deploy Settlement
    const Settlement = await ethers.getContractFactory("Settlement");
    settlement = await Settlement.deploy(
      owner.address,
      await vault.getAddress(),
      operator.address,
      feeRecipient.address
    );
    await settlement.waitForDeployment();

    // Set settlement contract in Vault
    await vault.connect(owner).setSettlementContract(await settlement.getAddress());

    // Mint tokens to users
    await baseToken.mint(maker.address, INITIAL_BALANCE);
    await baseToken.mint(taker.address, INITIAL_BALANCE);
    await quoteToken.mint(maker.address, INITIAL_BALANCE);
    await quoteToken.mint(taker.address, INITIAL_BALANCE);

    // Approve vault
    const vaultAddress = await vault.getAddress();
    await baseToken.connect(maker).approve(vaultAddress, ethers.MaxUint256);
    await baseToken.connect(taker).approve(vaultAddress, ethers.MaxUint256);
    await quoteToken.connect(maker).approve(vaultAddress, ethers.MaxUint256);
    await quoteToken.connect(taker).approve(vaultAddress, ethers.MaxUint256);

    // Deposit tokens to vault
    await vault.connect(maker).deposit(await baseToken.getAddress(), ethers.parseEther("1000"));
    await vault.connect(maker).deposit(await quoteToken.getAddress(), ethers.parseEther("1000"));
    await vault.connect(taker).deposit(await baseToken.getAddress(), ethers.parseEther("1000"));
    await vault.connect(taker).deposit(await quoteToken.getAddress(), ethers.parseEther("1000"));
  });

  describe("Deployment", function () {
    it("should set the correct vault", async function () {
      expect(await settlement.vault()).to.equal(await vault.getAddress());
    });

    it("should set the correct operator", async function () {
      expect(await settlement.operator()).to.equal(operator.address);
    });

    it("should set the correct fee recipient", async function () {
      expect(await settlement.feeRecipient()).to.equal(feeRecipient.address);
    });

    it("should revert with zero vault address", async function () {
      const Settlement = await ethers.getContractFactory("Settlement");
      await expect(
        Settlement.deploy(owner.address, ethers.ZeroAddress, operator.address, feeRecipient.address)
      ).to.be.revertedWithCustomError(settlement, "InvalidAddress");
    });
  });

  describe("Single Trade Settlement", function () {
    async function signTrade(trade: any): Promise<string> {
      const settlementAddress = await settlement.getAddress();
      const chainId = (await ethers.provider.getNetwork()).chainId;

      const domain = {
        name: DOMAIN_NAME,
        version: DOMAIN_VERSION,
        chainId: chainId,
        verifyingContract: settlementAddress,
      };

      const types = {
        Trade: [
          { name: "tradeId", type: "bytes32" },
          { name: "maker", type: "address" },
          { name: "taker", type: "address" },
          { name: "baseToken", type: "address" },
          { name: "quoteToken", type: "address" },
          { name: "baseAmount", type: "uint256" },
          { name: "quoteAmount", type: "uint256" },
          { name: "makerFee", type: "uint256" },
          { name: "takerFee", type: "uint256" },
          { name: "isBuy", type: "bool" },
        ],
      };

      return await operator.signTypedData(domain, types, trade);
    }

    it("should settle a single trade (maker buys)", async function () {
      const trade = {
        tradeId: ethers.id("trade-1"),
        maker: maker.address,
        taker: taker.address,
        baseToken: await baseToken.getAddress(),
        quoteToken: await quoteToken.getAddress(),
        baseAmount: ethers.parseEther("10"),
        quoteAmount: ethers.parseEther("100"),
        makerFee: ethers.parseEther("0.1"),
        takerFee: ethers.parseEther("0.1"),
        isBuy: true, // maker buys base, sells quote
      };

      // Lock funds for trade
      await vault.connect(operator).lockFunds(
        maker.address,
        await quoteToken.getAddress(),
        trade.quoteAmount + trade.makerFee,
        trade.tradeId
      );
      await vault.connect(operator).lockFunds(
        taker.address,
        await baseToken.getAddress(),
        trade.baseAmount,
        trade.tradeId
      );
      await vault.connect(operator).lockFunds(
        taker.address,
        await quoteToken.getAddress(),
        trade.takerFee,
        trade.tradeId
      );

      const signature = await signTrade(trade);

      await expect(settlement.settleSingleTrade(trade, signature))
        .to.emit(settlement, "TradeSettled");

      // Check balances after settlement
      // Maker received base token
      expect(await vault.getTotalBalance(maker.address, await baseToken.getAddress())).to.equal(
        ethers.parseEther("1010") // 1000 + 10
      );
      // Maker paid quote token + fee
      expect(await vault.getTotalBalance(maker.address, await quoteToken.getAddress())).to.equal(
        ethers.parseEther("899.9") // 1000 - 100 - 0.1
      );
      // Taker received quote token
      expect(await vault.getTotalBalance(taker.address, await quoteToken.getAddress())).to.equal(
        ethers.parseEther("1099.9") // 1000 + 100 - 0.1
      );
      // Taker paid base token
      expect(await vault.getTotalBalance(taker.address, await baseToken.getAddress())).to.equal(
        ethers.parseEther("990") // 1000 - 10
      );
      // Fee recipient received fees
      expect(await vault.getTotalBalance(feeRecipient.address, await quoteToken.getAddress())).to.equal(
        ethers.parseEther("0.2") // 0.1 + 0.1
      );
    });

    it("should settle a single trade (maker sells)", async function () {
      const trade = {
        tradeId: ethers.id("trade-2"),
        maker: maker.address,
        taker: taker.address,
        baseToken: await baseToken.getAddress(),
        quoteToken: await quoteToken.getAddress(),
        baseAmount: ethers.parseEther("10"),
        quoteAmount: ethers.parseEther("100"),
        makerFee: ethers.parseEther("0.1"),
        takerFee: ethers.parseEther("0.1"),
        isBuy: false, // maker sells base, receives quote
      };

      // Lock funds for trade
      await vault.connect(operator).lockFunds(
        maker.address,
        await baseToken.getAddress(),
        trade.baseAmount,
        trade.tradeId
      );
      await vault.connect(operator).lockFunds(
        maker.address,
        await quoteToken.getAddress(),
        trade.makerFee,
        trade.tradeId
      );
      await vault.connect(operator).lockFunds(
        taker.address,
        await quoteToken.getAddress(),
        trade.quoteAmount + trade.takerFee,
        trade.tradeId
      );

      const signature = await signTrade(trade);

      await settlement.settleSingleTrade(trade, signature);

      // Check trade is settled
      expect(await settlement.isTradeSettled(trade.tradeId)).to.be.true;
    });

    it("should revert with invalid signature", async function () {
      const trade = {
        tradeId: ethers.id("trade-3"),
        maker: maker.address,
        taker: taker.address,
        baseToken: await baseToken.getAddress(),
        quoteToken: await quoteToken.getAddress(),
        baseAmount: ethers.parseEther("10"),
        quoteAmount: ethers.parseEther("100"),
        makerFee: 0,
        takerFee: 0,
        isBuy: true,
      };

      // Sign with wrong signer
      const wrongSignature = await maker.signMessage("wrong");

      await expect(settlement.settleSingleTrade(trade, wrongSignature)).to.be.revertedWithCustomError(
        settlement,
        "InvalidSignature"
      );
    });

    it("should revert settling same trade twice", async function () {
      const trade = {
        tradeId: ethers.id("trade-4"),
        maker: maker.address,
        taker: taker.address,
        baseToken: await baseToken.getAddress(),
        quoteToken: await quoteToken.getAddress(),
        baseAmount: ethers.parseEther("10"),
        quoteAmount: ethers.parseEther("100"),
        makerFee: 0,
        takerFee: 0,
        isBuy: true,
      };

      await vault.connect(operator).lockFunds(
        maker.address,
        await quoteToken.getAddress(),
        trade.quoteAmount,
        trade.tradeId
      );
      await vault.connect(operator).lockFunds(
        taker.address,
        await baseToken.getAddress(),
        trade.baseAmount,
        trade.tradeId
      );

      const signature = await signTrade(trade);

      await settlement.settleSingleTrade(trade, signature);

      await expect(settlement.settleSingleTrade(trade, signature)).to.be.revertedWithCustomError(
        settlement,
        "TradeAlreadySettled"
      );
    });
  });

  describe("Batch Settlement", function () {
    async function signBatch(batch: any): Promise<string> {
      const settlementAddress = await settlement.getAddress();
      const chainId = (await ethers.provider.getNetwork()).chainId;

      // Calculate trades hash - same as Solidity _hashBatch
      const tradeHashes: string[] = [];
      for (const trade of batch.trades) {
        const tradeHash = ethers.keccak256(
          ethers.AbiCoder.defaultAbiCoder().encode(
            [
              "bytes32",
              "bytes32",
              "address",
              "address",
              "address",
              "address",
              "uint256",
              "uint256",
              "uint256",
              "uint256",
              "bool",
            ],
            [
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
              trade.isBuy,
            ]
          )
        );
        tradeHashes.push(tradeHash);
      }
      // Use abi.encodePacked for array of bytes32
      const tradesHash = ethers.keccak256(ethers.concat(tradeHashes));

      // Use EIP-712 typed data signing
      const domain = {
        name: DOMAIN_NAME,
        version: DOMAIN_VERSION,
        chainId: chainId,
        verifyingContract: settlementAddress,
      };

      const types = {
        SettlementBatch: [
          { name: "batchId", type: "bytes32" },
          { name: "tradesHash", type: "bytes32" },
          { name: "timestamp", type: "uint256" },
        ],
      };

      const value = {
        batchId: batch.batchId,
        tradesHash: tradesHash,
        timestamp: batch.timestamp,
      };

      return await operator.signTypedData(domain, types, value);
    }

    it("should settle a batch of trades", async function () {
      const trades = [
        {
          tradeId: ethers.id("batch-trade-1"),
          maker: maker.address,
          taker: taker.address,
          baseToken: await baseToken.getAddress(),
          quoteToken: await quoteToken.getAddress(),
          baseAmount: ethers.parseEther("5"),
          quoteAmount: ethers.parseEther("50"),
          makerFee: 0,
          takerFee: 0,
          isBuy: true,
        },
        {
          tradeId: ethers.id("batch-trade-2"),
          maker: maker.address,
          taker: taker.address,
          baseToken: await baseToken.getAddress(),
          quoteToken: await quoteToken.getAddress(),
          baseAmount: ethers.parseEther("3"),
          quoteAmount: ethers.parseEther("30"),
          makerFee: 0,
          takerFee: 0,
          isBuy: true,
        },
      ];

      // Lock funds for all trades
      for (const trade of trades) {
        await vault.connect(operator).lockFunds(
          maker.address,
          await quoteToken.getAddress(),
          trade.quoteAmount,
          trade.tradeId
        );
        await vault.connect(operator).lockFunds(
          taker.address,
          await baseToken.getAddress(),
          trade.baseAmount,
          trade.tradeId
        );
      }

      const batch = {
        batchId: ethers.id("batch-1"),
        trades: trades,
        timestamp: Math.floor(Date.now() / 1000),
        operatorSig: "0x", // will be replaced
      };

      batch.operatorSig = await signBatch(batch);

      await expect(settlement.settleBatch(batch)).to.emit(settlement, "BatchSettled");

      // Check batch is settled
      expect(await settlement.isBatchSettled(batch.batchId)).to.be.true;

      // Check all trades are settled
      for (const trade of trades) {
        expect(await settlement.isTradeSettled(trade.tradeId)).to.be.true;
      }
    });

    it("should revert empty batch", async function () {
      const batch = {
        batchId: ethers.id("empty-batch"),
        trades: [],
        timestamp: Math.floor(Date.now() / 1000),
        operatorSig: "0x",
      };

      await expect(settlement.settleBatch(batch)).to.be.revertedWithCustomError(settlement, "EmptyBatch");
    });

    it("should revert settling same batch twice", async function () {
      const trades = [
        {
          tradeId: ethers.id("batch-trade-dup"),
          maker: maker.address,
          taker: taker.address,
          baseToken: await baseToken.getAddress(),
          quoteToken: await quoteToken.getAddress(),
          baseAmount: ethers.parseEther("5"),
          quoteAmount: ethers.parseEther("50"),
          makerFee: 0,
          takerFee: 0,
          isBuy: true,
        },
      ];

      await vault.connect(operator).lockFunds(
        maker.address,
        await quoteToken.getAddress(),
        trades[0].quoteAmount,
        trades[0].tradeId
      );
      await vault.connect(operator).lockFunds(
        taker.address,
        await baseToken.getAddress(),
        trades[0].baseAmount,
        trades[0].tradeId
      );

      const batch = {
        batchId: ethers.id("batch-dup"),
        trades: trades,
        timestamp: Math.floor(Date.now() / 1000),
        operatorSig: "0x",
      };

      batch.operatorSig = await signBatch(batch);

      await settlement.settleBatch(batch);

      await expect(settlement.settleBatch(batch)).to.be.revertedWithCustomError(
        settlement,
        "BatchAlreadySettled"
      );
    });
  });

  describe("Admin Functions", function () {
    it("should allow owner to update operator", async function () {
      await expect(settlement.connect(owner).setOperator(maker.address))
        .to.emit(settlement, "OperatorUpdated")
        .withArgs(operator.address, maker.address);

      expect(await settlement.operator()).to.equal(maker.address);
    });

    it("should allow owner to update fee recipient", async function () {
      await expect(settlement.connect(owner).setFeeRecipient(maker.address))
        .to.emit(settlement, "FeeRecipientUpdated")
        .withArgs(feeRecipient.address, maker.address);

      expect(await settlement.feeRecipient()).to.equal(maker.address);
    });

    it("should allow owner to update vault", async function () {
      const newVault = await (await ethers.getContractFactory("Vault")).deploy(owner.address, operator.address);

      await expect(settlement.connect(owner).setVault(await newVault.getAddress()))
        .to.emit(settlement, "VaultUpdated")
        .withArgs(await vault.getAddress(), await newVault.getAddress());

      expect(await settlement.vault()).to.equal(await newVault.getAddress());
    });

    it("should revert non-owner admin calls", async function () {
      await expect(settlement.connect(maker).setOperator(maker.address)).to.be.revertedWithCustomError(
        settlement,
        "OwnableUnauthorizedAccount"
      );
    });
  });

  describe("View Functions", function () {
    it("should track total fees collected", async function () {
      const trade = {
        tradeId: ethers.id("fee-trade"),
        maker: maker.address,
        taker: taker.address,
        baseToken: await baseToken.getAddress(),
        quoteToken: await quoteToken.getAddress(),
        baseAmount: ethers.parseEther("10"),
        quoteAmount: ethers.parseEther("100"),
        makerFee: ethers.parseEther("1"),
        takerFee: ethers.parseEther("0.5"),
        isBuy: true,
      };

      await vault.connect(operator).lockFunds(
        maker.address,
        await quoteToken.getAddress(),
        trade.quoteAmount + trade.makerFee,
        trade.tradeId
      );
      await vault.connect(operator).lockFunds(
        taker.address,
        await baseToken.getAddress(),
        trade.baseAmount,
        trade.tradeId
      );
      await vault.connect(operator).lockFunds(
        taker.address,
        await quoteToken.getAddress(),
        trade.takerFee,
        trade.tradeId
      );

      // Sign trade
      const settlementAddress = await settlement.getAddress();
      const chainId = (await ethers.provider.getNetwork()).chainId;

      const domain = {
        name: DOMAIN_NAME,
        version: DOMAIN_VERSION,
        chainId: chainId,
        verifyingContract: settlementAddress,
      };

      const types = {
        Trade: [
          { name: "tradeId", type: "bytes32" },
          { name: "maker", type: "address" },
          { name: "taker", type: "address" },
          { name: "baseToken", type: "address" },
          { name: "quoteToken", type: "address" },
          { name: "baseAmount", type: "uint256" },
          { name: "quoteAmount", type: "uint256" },
          { name: "makerFee", type: "uint256" },
          { name: "takerFee", type: "uint256" },
          { name: "isBuy", type: "bool" },
        ],
      };

      const signature = await operator.signTypedData(domain, types, trade);

      await settlement.settleSingleTrade(trade, signature);

      expect(await settlement.totalFeesCollected(await quoteToken.getAddress())).to.equal(
        ethers.parseEther("1.5")
      );
    });
  });
});
