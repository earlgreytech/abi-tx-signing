# Golang functions to create and sign transactions (p2pkh and contract interactions) locally
#Table of Contents
- [Requirements](#requirements)
- [Create a P2PKH Tx (send to another address)](#P2PKH)
- [Deploy a Contract or Interact with an existing one](#Contract Creation & Interaction)

## Requirements
- Golang
- janus, qtum-cli, or any other method to broadcast a transaction to a testchain or mainnet

## P2PKH

```
func P2khTx(privKey string, destination string, amount int64) (string, error)
```
Create a p2pkh raw transaction to send qtum to another address.
- privKey is the private key associated with the address where the funds come from and it is also used to sign the Tx
- destination is the base58 encoded address to send the qtum to
- amount is the amount in qtum to send to the destination address expressed in satoshis (10^8)
```
//(rawTx is a hex string representing the raw signed tx created)
rawTx, err := P2khTx("cMbgxCJrTYUqgcmiC1berh5DFrtY1KeU4PXZ6NZxgenniF1mXCRk", "qLn9vqbr2Gx3TsVR9QyTVB5mrMoh4x43Uf", 200000000)
if err != nil {
 fmt.Println("Err coming from CreateTx")
 fmt.Println(err)
}
```
Send the rawTx through qtum 
```
qtum-cli -rpcuser=qtum -rpcpassword=testpasswd sendrawtransaction rawTx
```

## Contract Creation & Interaction

```
func ContractTx(privKey string, from string, contractAddr string, amount int64, data []byte, gas int64, gasPrice int64, opcode byte) (string, error
```
Create a Tx to deploy a contract or to interact with an already deployed one
- privKey is the private key associated with the address where the funds come from and it is also used to sign the Tx
- from is the address of the contract creator/sender (should be the one associated with the privKey)
- contractAddr is the address of the contract to be interacted with (empty string if deploying a contract)
- amount is the amount to be spent deploying or calling the contract
- data is either the bytecode of the compiled contract used to deploy it, or the abi encoded data to used to interact with a contract
- gas is gas units to be used
- gasPrice is the price of gas
- opcode is either OP_CALL or OP_CREATE

To create a contract
```
rawTx, err := ContractTx("cMbgxCJrTYUqgcmiC1berh5DFrtY1KeU4PXZ6NZxgenniF1mXCRk", "qUbxboqjBRp96j3La8D1RYkyqx5uQbJPoW", "", 2000000000, contractByteCode, 2500000, 40, OP_CREATE)
	if err != nil {
		fmt.Println("Err coming from ContractTx")
		fmt.Println(err)
	}
```

To call a contract
```
/*
  Example for the SimpleStore.sol contract
*/
//Contract's ABI in JSON format
var abiJson = `[{"inputs":[],"name":"get","outputs":[{"internalType":"uint256","name":"","type":"uint256"}],"stateMutability":"view","type":"function"},{"inputs":[{"internalType":"uint256","name":"x","type":"uint256"}],"name":"set","outputs":[],"stateMutability":"nonpayable","type":"function"}]`

//Arguments to functions used to interact with a contract
arguments := map[string][]interface{}{
 "set": {big.NewInt(5)},
}

//CallContractData takes an io.Reader used to read the contract's abi, and a list of arguments of type map[string][]interface{}, where the key is the name of the method
//and the values are the inputs to be packed to generate the callData of type []byte
callData, err := CallContractData(strings.NewReader(abiJson), arguments) 
if err != nil {
		fmt.Println("Could Not Pack arguments to form callData")
		fmt.Println(err)
}

rawTx, err := ContractTx("cMbgxCJrTYUqgcmiC1berh5DFrtY1KeU4PXZ6NZxgenniF1mXCRk", "qUbxboqjBRp96j3La8D1RYkyqx5uQbJPoW", "dcb58d4670a6922abc89d5fc1aea38316ee7e373", 2000000000, callData, 2500000, 40, OP_CALL)
	if err != nil {
		fmt.Println("Err coming from ContractTx")
		fmt.Println(err)
	}
```

Again, you can broadcast the resulting hex to a network
```
qtum-cli -rpcuser=qtum -rpcpassword=testpasswd sendrawtransaction rawTx
```
