// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

/// @title SignatureVerifier - ECDSA signature verification utilities
library SignatureVerifier {
    error InvalidSignatureLength();
    error InvalidSignatureS();

    /// @notice Recover the signer of a message hash
    /// @param hash The message hash
    /// @param signature The signature bytes
    /// @return The recovered signer address
    function recover(bytes32 hash, bytes memory signature) internal pure returns (address) {
        if (signature.length != 65) {
            revert InvalidSignatureLength();
        }

        bytes32 r;
        bytes32 s;
        uint8 v;

        assembly {
            r := mload(add(signature, 0x20))
            s := mload(add(signature, 0x40))
            v := byte(0, mload(add(signature, 0x60)))
        }

        // EIP-2 still allows signature malleability for ecrecover(). Remove this possibility and make the signature
        // unique. Appendix F in the Ethereum Yellow paper (https://ethereum.github.io/yellowpaper/paper.pdf), defines
        // the valid range for s in (301): 0 < s < secp256k1n / 2 + 1, and for v in (302): v in {27, 28}.
        if (uint256(s) > 0x7FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF5D576E7357A4501DDFE92F46681B20A0) {
            revert InvalidSignatureS();
        }

        if (v < 27) {
            v += 27;
        }

        return ecrecover(hash, v, r, s);
    }

    /// @notice Create an Ethereum signed message hash
    /// @param hash The message hash
    /// @return The Ethereum signed message hash
    function toEthSignedMessageHash(bytes32 hash) internal pure returns (bytes32) {
        return keccak256(abi.encodePacked("\x19Ethereum Signed Message:\n32", hash));
    }

    /// @notice Create a typed data hash (EIP-712)
    /// @param domainSeparator The domain separator
    /// @param structHash The struct hash
    /// @return The typed data hash
    function toTypedDataHash(bytes32 domainSeparator, bytes32 structHash) internal pure returns (bytes32) {
        return keccak256(abi.encodePacked("\x19\x01", domainSeparator, structHash));
    }
}
