import { ethers } from "hardhat";

async function main() {
  const [deployer] = await ethers.getSigners();
  console.log("Deploying contracts with account:", deployer.address);

  const balance = await ethers.provider.getBalance(deployer.address);
  console.log("Account balance:", ethers.formatEther(balance), "ETH");

  // Get deployment configuration from environment
  const ownerAddress = process.env.OWNER_ADDRESS || deployer.address;
  const operatorAddress = process.env.OPERATOR_ADDRESS || deployer.address;
  const feeRecipientAddress = process.env.FEE_RECIPIENT_ADDRESS || deployer.address;

  console.log("\nDeployment Configuration:");
  console.log("  Owner:", ownerAddress);
  console.log("  Operator:", operatorAddress);
  console.log("  Fee Recipient:", feeRecipientAddress);

  // Deploy Vault
  console.log("\n1. Deploying Vault...");
  const Vault = await ethers.getContractFactory("Vault");
  const vault = await Vault.deploy(ownerAddress, operatorAddress);
  await vault.waitForDeployment();
  const vaultAddress = await vault.getAddress();
  console.log("   Vault deployed to:", vaultAddress);

  // Deploy Settlement
  console.log("\n2. Deploying Settlement...");
  const Settlement = await ethers.getContractFactory("Settlement");
  const settlement = await Settlement.deploy(
    ownerAddress,
    vaultAddress,
    operatorAddress,
    feeRecipientAddress
  );
  await settlement.waitForDeployment();
  const settlementAddress = await settlement.getAddress();
  console.log("   Settlement deployed to:", settlementAddress);

  // Set settlement contract in Vault
  console.log("\n3. Configuring Vault...");
  const vaultWithSigner = vault.connect(deployer);
  const tx = await vaultWithSigner.setSettlementContract(settlementAddress);
  await tx.wait();
  console.log("   Settlement contract set in Vault");

  // Summary
  console.log("\n========================================");
  console.log("Deployment Complete!");
  console.log("========================================");
  console.log("\nContract Addresses:");
  console.log("  Vault:", vaultAddress);
  console.log("  Settlement:", settlementAddress);
  console.log("\nConfiguration:");
  console.log("  Owner:", ownerAddress);
  console.log("  Operator:", operatorAddress);
  console.log("  Fee Recipient:", feeRecipientAddress);
  console.log("========================================\n");

  // Return addresses for verification
  return {
    vault: vaultAddress,
    settlement: settlementAddress,
  };
}

main()
  .then((addresses) => {
    console.log("Addresses for verification:", addresses);
    process.exit(0);
  })
  .catch((error) => {
    console.error(error);
    process.exit(1);
  });
