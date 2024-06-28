// SPDX-License-Identifier: MIT
pragma solidity ^0.6.0;
pragma experimental ABIEncoderV2;


contract BalanceChecker {

    function balance(address[] memory tokens) view public returns (bool[] memory) {
        bool[] memory results = new bool[](tokens.length);
        for (uint i=0;i<tokens.length;i++) {
            (bool success, ) = tokens[i].staticcall(abi.encodeWithSignature("balanceOf(address)", address(this)));
            results[i] = success;
        }
        return results;
    }

    function _decodeBalance(bytes memory data) public pure returns (uint256) {
        return abi.decode(data, (uint256));
    }

    function tokenBalance(address[] memory tokens, address[][] memory users) view public returns (uint256[][] memory) {
        uint256[][] memory userBalances = new uint256[][](tokens.length);
        for (uint256 i = 0; i < tokens.length; i++) {
            address[] memory tokenUsers = users[i];
            uint256[] memory balances = new uint256[](tokenUsers.length);
            for (uint256 j = 0; j < tokenUsers.length; j++) {
                (bool success, bytes memory data) = tokens[i].staticcall(abi.encodeWithSignature("balanceOf(address)", tokenUsers[j]));
                if (success) {
                    if (data.length == 0) {
                        balances[j] = 0;
                    } else {
                        try this._decodeBalance(data) returns (uint256 bal) {
                            balances[j] = bal;
                        } catch {
                            balances[j] = 0;
                        }
                    }
                } else {
                    balances[j] = 0;
                }
            }
            userBalances[i] = balances;
        }
        return userBalances;
    }
}
