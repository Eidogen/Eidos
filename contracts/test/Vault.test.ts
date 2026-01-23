import { expect } from "chai";
import { ethers } from "hardhat";
import { SignerWithAddress } from "@nomicfoundation/hardhat-ethers/signers";
import { Vault, MockERC20 } from "../typechain-types";

describe("Vault", function () {
  let vault: Vault;
  let token: MockERC20;
  let owner: SignerWithAddress;
  let operator: SignerWithAddress;
  let settlement: SignerWithAddress;
  let user1: SignerWithAddress;
  let user2: SignerWithAddress;

  const ETH_ADDRESS = ethers.ZeroAddress;
  const INITIAL_BALANCE = ethers.parseEther("1000");

  beforeEach(async function () {
    [owner, operator, settlement, user1, user2] = await ethers.getSigners();

    // Deploy MockERC20
    const MockERC20 = await ethers.getContractFactory("MockERC20");
    token = await MockERC20.deploy("Test Token", "TEST", 18);
    await token.waitForDeployment();

    // Deploy Vault
    const Vault = await ethers.getContractFactory("Vault");
    vault = await Vault.deploy(owner.address, operator.address);
    await vault.waitForDeployment();

    // Set settlement contract
    await vault.connect(owner).setSettlementContract(settlement.address);

    // Mint tokens to users
    await token.mint(user1.address, INITIAL_BALANCE);
    await token.mint(user2.address, INITIAL_BALANCE);

    // Approve vault
    await token.connect(user1).approve(await vault.getAddress(), ethers.MaxUint256);
    await token.connect(user2).approve(await vault.getAddress(), ethers.MaxUint256);
  });

  describe("Deployment", function () {
    it("should set the correct owner", async function () {
      expect(await vault.owner()).to.equal(owner.address);
    });

    it("should set the correct operator", async function () {
      expect(await vault.operator()).to.equal(operator.address);
    });

    it("should revert with zero operator address", async function () {
      const Vault = await ethers.getContractFactory("Vault");
      await expect(
        Vault.deploy(owner.address, ethers.ZeroAddress)
      ).to.be.revertedWithCustomError(vault, "InvalidAddress");
    });
  });

  describe("ERC20 Deposits", function () {
    it("should allow depositing tokens", async function () {
      const amount = ethers.parseEther("100");

      await expect(vault.connect(user1).deposit(await token.getAddress(), amount))
        .to.emit(vault, "Deposit")
        .withArgs(user1.address, await token.getAddress(), amount);

      expect(await vault.getTotalBalance(user1.address, await token.getAddress())).to.equal(amount);
      expect(await vault.getAvailableBalance(user1.address, await token.getAddress())).to.equal(amount);
    });

    it("should revert deposit with zero amount", async function () {
      await expect(
        vault.connect(user1).deposit(await token.getAddress(), 0)
      ).to.be.revertedWithCustomError(vault, "InvalidAmount");
    });

    it("should revert deposit with zero token address", async function () {
      await expect(
        vault.connect(user1).deposit(ethers.ZeroAddress, ethers.parseEther("100"))
      ).to.be.revertedWithCustomError(vault, "InvalidAddress");
    });
  });

  describe("ETH Deposits", function () {
    it("should allow depositing ETH", async function () {
      const amount = ethers.parseEther("1");

      await expect(vault.connect(user1).depositETH({ value: amount }))
        .to.emit(vault, "Deposit")
        .withArgs(user1.address, ETH_ADDRESS, amount);

      expect(await vault.getTotalBalance(user1.address, ETH_ADDRESS)).to.equal(amount);
    });

    it("should allow depositing ETH via receive", async function () {
      const amount = ethers.parseEther("1");

      await expect(
        user1.sendTransaction({ to: await vault.getAddress(), value: amount })
      ).to.emit(vault, "Deposit").withArgs(user1.address, ETH_ADDRESS, amount);

      expect(await vault.getTotalBalance(user1.address, ETH_ADDRESS)).to.equal(amount);
    });
  });

  describe("ERC20 Withdrawals", function () {
    beforeEach(async function () {
      await vault.connect(user1).deposit(await token.getAddress(), ethers.parseEther("100"));
    });

    it("should allow withdrawing tokens", async function () {
      const amount = ethers.parseEther("50");

      await expect(vault.connect(user1).withdraw(await token.getAddress(), amount))
        .to.emit(vault, "Withdraw")
        .withArgs(user1.address, await token.getAddress(), amount);

      expect(await vault.getTotalBalance(user1.address, await token.getAddress())).to.equal(
        ethers.parseEther("50")
      );
    });

    it("should revert withdrawal exceeding available balance", async function () {
      await expect(
        vault.connect(user1).withdraw(await token.getAddress(), ethers.parseEther("150"))
      ).to.be.revertedWithCustomError(vault, "InsufficientBalance");
    });
  });

  describe("ETH Withdrawals", function () {
    beforeEach(async function () {
      await vault.connect(user1).depositETH({ value: ethers.parseEther("1") });
    });

    it("should allow withdrawing ETH", async function () {
      const amount = ethers.parseEther("0.5");

      await expect(vault.connect(user1).withdrawETH(amount))
        .to.emit(vault, "Withdraw")
        .withArgs(user1.address, ETH_ADDRESS, amount);
    });

    it("should revert withdrawal exceeding available balance", async function () {
      await expect(
        vault.connect(user1).withdrawETH(ethers.parseEther("2"))
      ).to.be.revertedWithCustomError(vault, "InsufficientBalance");
    });
  });

  describe("Lock Funds", function () {
    const depositAmount = ethers.parseEther("100");
    const lockAmount = ethers.parseEther("50");
    const orderId = ethers.id("order-1");

    beforeEach(async function () {
      await vault.connect(user1).deposit(await token.getAddress(), depositAmount);
    });

    it("should allow operator to lock funds", async function () {
      await expect(
        vault.connect(operator).lockFunds(user1.address, await token.getAddress(), lockAmount, orderId)
      )
        .to.emit(vault, "FundsLocked")
        .withArgs(user1.address, await token.getAddress(), lockAmount, orderId);

      expect(await vault.getLockedBalance(user1.address, await token.getAddress())).to.equal(lockAmount);
      expect(await vault.getAvailableBalance(user1.address, await token.getAddress())).to.equal(
        depositAmount - lockAmount
      );
    });

    it("should revert locking by non-operator", async function () {
      await expect(
        vault.connect(user1).lockFunds(user1.address, await token.getAddress(), lockAmount, orderId)
      ).to.be.revertedWithCustomError(vault, "Unauthorized");
    });

    it("should revert locking more than available", async function () {
      await expect(
        vault.connect(operator).lockFunds(
          user1.address,
          await token.getAddress(),
          ethers.parseEther("150"),
          orderId
        )
      ).to.be.revertedWithCustomError(vault, "InsufficientBalance");
    });

    it("should prevent withdrawal of locked funds", async function () {
      await vault.connect(operator).lockFunds(user1.address, await token.getAddress(), lockAmount, orderId);

      await expect(
        vault.connect(user1).withdraw(await token.getAddress(), depositAmount)
      ).to.be.revertedWithCustomError(vault, "InsufficientBalance");
    });
  });

  describe("Unlock Funds", function () {
    const depositAmount = ethers.parseEther("100");
    const lockAmount = ethers.parseEther("50");
    const orderId = ethers.id("order-1");

    beforeEach(async function () {
      await vault.connect(user1).deposit(await token.getAddress(), depositAmount);
      await vault.connect(operator).lockFunds(user1.address, await token.getAddress(), lockAmount, orderId);
    });

    it("should allow operator to unlock funds", async function () {
      await expect(
        vault.connect(operator).unlockFunds(user1.address, await token.getAddress(), lockAmount, orderId)
      )
        .to.emit(vault, "FundsUnlocked")
        .withArgs(user1.address, await token.getAddress(), lockAmount, orderId);

      expect(await vault.getLockedBalance(user1.address, await token.getAddress())).to.equal(0);
      expect(await vault.getAvailableBalance(user1.address, await token.getAddress())).to.equal(depositAmount);
    });

    it("should revert unlocking more than locked", async function () {
      await expect(
        vault.connect(operator).unlockFunds(
          user1.address,
          await token.getAddress(),
          ethers.parseEther("100"),
          orderId
        )
      ).to.be.revertedWithCustomError(vault, "InsufficientLockedBalance");
    });
  });

  describe("Settlement Transfer", function () {
    const depositAmount = ethers.parseEther("100");
    const lockAmount = ethers.parseEther("50");
    const orderId = ethers.id("order-1");
    const settlementId = ethers.id("settlement-1");

    beforeEach(async function () {
      await vault.connect(user1).deposit(await token.getAddress(), depositAmount);
      await vault.connect(operator).lockFunds(user1.address, await token.getAddress(), lockAmount, orderId);
    });

    it("should allow settlement contract to transfer", async function () {
      const transferAmount = ethers.parseEther("30");

      await expect(
        vault.connect(settlement).settlementTransfer(
          user1.address,
          user2.address,
          await token.getAddress(),
          transferAmount,
          settlementId
        )
      )
        .to.emit(vault, "SettlementTransfer")
        .withArgs(user1.address, user2.address, await token.getAddress(), transferAmount, settlementId);

      expect(await vault.getTotalBalance(user1.address, await token.getAddress())).to.equal(
        depositAmount - transferAmount
      );
      expect(await vault.getTotalBalance(user2.address, await token.getAddress())).to.equal(transferAmount);
      expect(await vault.getLockedBalance(user1.address, await token.getAddress())).to.equal(
        lockAmount - transferAmount
      );
    });

    it("should revert transfer by non-settlement", async function () {
      await expect(
        vault.connect(user1).settlementTransfer(
          user1.address,
          user2.address,
          await token.getAddress(),
          ethers.parseEther("30"),
          settlementId
        )
      ).to.be.revertedWithCustomError(vault, "Unauthorized");
    });

    it("should revert transfer exceeding locked balance", async function () {
      await expect(
        vault.connect(settlement).settlementTransfer(
          user1.address,
          user2.address,
          await token.getAddress(),
          ethers.parseEther("60"),
          settlementId
        )
      ).to.be.revertedWithCustomError(vault, "InsufficientLockedBalance");
    });
  });

  describe("Admin Functions", function () {
    it("should allow owner to set operator", async function () {
      await expect(vault.connect(owner).setOperator(user1.address))
        .to.emit(vault, "OperatorUpdated")
        .withArgs(operator.address, user1.address);

      expect(await vault.operator()).to.equal(user1.address);
    });

    it("should revert non-owner setting operator", async function () {
      await expect(vault.connect(user1).setOperator(user1.address)).to.be.revertedWithCustomError(
        vault,
        "OwnableUnauthorizedAccount"
      );
    });

    it("should allow owner to add supported token", async function () {
      await vault.connect(owner).setTokenWhitelistEnabled(true);
      await vault.connect(owner).addSupportedToken(await token.getAddress());

      expect(await vault.isTokenSupported(await token.getAddress())).to.be.true;
    });

    it("should allow owner to enable token whitelist", async function () {
      await vault.connect(owner).setTokenWhitelistEnabled(true);
      expect(await vault.tokenWhitelistEnabled()).to.be.true;
    });

    it("should reject unsupported token when whitelist enabled", async function () {
      await vault.connect(owner).setTokenWhitelistEnabled(true);

      await expect(
        vault.connect(user1).deposit(await token.getAddress(), ethers.parseEther("100"))
      ).to.be.revertedWithCustomError(vault, "TokenNotSupported");
    });
  });

  describe("Emergency Withdraw", function () {
    it("should allow owner to emergency withdraw", async function () {
      await vault.connect(user1).deposit(await token.getAddress(), ethers.parseEther("100"));

      await vault
        .connect(owner)
        .emergencyWithdraw(await token.getAddress(), ethers.parseEther("100"), owner.address);

      expect(await token.balanceOf(owner.address)).to.equal(ethers.parseEther("100"));
    });

    it("should allow owner to emergency withdraw ETH", async function () {
      await vault.connect(user1).depositETH({ value: ethers.parseEther("1") });

      const balanceBefore = await ethers.provider.getBalance(owner.address);

      await vault.connect(owner).emergencyWithdraw(ETH_ADDRESS, ethers.parseEther("1"), owner.address);

      const balanceAfter = await ethers.provider.getBalance(owner.address);
      expect(balanceAfter).to.be.gt(balanceBefore);
    });
  });
});
