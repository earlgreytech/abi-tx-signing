package qtumtxsigner

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"

	"github.com/decred/dcrd/dcrec/secp256k1/v3"
	"github.com/decred/dcrd/dcrec/secp256k1/v3/ecdsa"
	"github.com/qtumproject/qtumsuite"
	"github.com/qtumproject/qtumsuite/chaincfg"
	"github.com/qtumproject/qtumsuite/chaincfg/chainhash"
	"github.com/qtumproject/qtumsuite/txscript"
	"github.com/qtumproject/qtumsuite/wire"
	"github.com/shopspring/decimal"
)

type ContractTransaction struct {
	From     string   `json:"from"`
	To       string   `json:"to"`
	Gas      *big.Int `json:"gas"`      // optional
	GasPrice *big.Int `json:"gasPrice"` // optional
	Value    string   `json:"value"`    // optional
	Data     string   `json:"data"`     // optional
	Nonce    string   `json:"nonce"`    // optional
}

type Transaction struct {
	TxID               string `json:"txid"`
	SourceAddress      string `json:"source_address"`
	DestinationAddress string `json:"destination_address"`
	Amount             int64  `json:"amount"`
	UnsignedTx         string `json:"unsignedtx"`
	SignedTx           string `json:"signedtx"`
}

type JSONRPCRequest struct {
	ID      string        `json:"id"`
	Jsonrpc string        `json:"jsonrpc"`
	Method  string        `json:"method"`
	Params  []interface{} `json:"params"`
}

type JSONRPCResult struct {
	JSONRPC   string          `json:"jsonrpc"`
	RawResult json.RawMessage `json:"result,omitempty"`
	Error     *JSONRPCError   `json:"error,omitempty"`
	ID        json.RawMessage `json:"id"`
}

type JSONRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type ListUnspentResponse []struct {
	Address       string          `json:"address"`
	Txid          string          `json:"txid"`
	Vout          uint            `json:"vout"`
	Amount        decimal.Decimal `json:"amount"`
	Safe          bool            `json:"safe"`
	Spendable     bool            `json:"spendable"`
	Solvable      bool            `json:"solvable"`
	Label         string          `json:"label"`
	Confirmations int             `json:"confirmations"`
	ScriptPubKey  string          `json:"scriptPubKey"`
	RedeemScript  string          `json:"redeemScript"`
}

//Take in an ABI in JSON format and return a the corresponding hex_string
/*func DecodeTx(data []byte) (string, error) {
	//Load ABI
	var abi *abi.ABI
	err := abi.UnmarshalJSON(data)
	if err != nil {
		return nil, err
	}

	//extract methods from the ABI

	return tx, nil

}*/

func GatherUTXOs(serilizedPubKey []byte, sourceTx *wire.MsgTx) (*ListUnspentResponse, int64, error) {

	//Get UTXOs from network
	//Use the UTXOs to figure out the previousTxId as well as the pubKeyScript
	/* LOOK INTO JANUS TAKING ADDRESSES WITHOUT THE 0x PREFIX AND STILL RETURNING A BALANCE*/
	keyid := qtumsuite.Hash160(serilizedPubKey)
	params := []interface{}{"0x" + hex.EncodeToString(keyid), 0.005}
	data := JSONRPCRequest{
		ID:      "10",
		Jsonrpc: "2.0",
		Method:  "qtum_getUTXOs",
		Params:  params,
	}

	payloadBytes, err := json.Marshal(data)
	if err != nil {
		return nil, 0, err
	}
	body := bytes.NewReader(payloadBytes)
	//Link to RPC
	req, err := http.NewRequest("POST", "http://localhost:23889", body)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	var cResp JSONRPCResult

	if err := json.NewDecoder(resp.Body).Decode(&cResp); err != nil {
		return nil, 0, err
	}

	var listUnspentResp *ListUnspentResponse

	if err := json.Unmarshal(cResp.RawResult, &listUnspentResp); err != nil {
		return nil, 0, err
	}

	balance := decimal.NewFromFloat(0)
	for _, utxo := range *listUnspentResp {
		balance = balance.Add(utxo.Amount)
	}

	balance = balance.Mul(decimal.NewFromFloat(1e8))
	floatBalance, exact := balance.Float64()

	if exact != true {
		return nil, 0, err
	}

	return listUnspentResp, int64(floatBalance), nil
}

func CreateTx(privKey string, destination string, amount int64) (string, error) {

	var qtumTestNetParams = chaincfg.MainNetParams
	//TestnetParams
	qtumTestNetParams.PubKeyHashAddrID = 120
	qtumTestNetParams.ScriptHashAddrID = 110

	redeemTx := wire.NewMsgTx(wire.TxVersion)

	//Decode WIF
	wif, err := qtumsuite.DecodeWIF(privKey)
	if err != nil {
		return "", err
	}

	//Gather info extracted from UTXOs related to addrPubKey (prevTxId, balance, pkScript)
	utxos, balance, err := GatherUTXOs(wif.SerializePubKey(), redeemTx)
	if err != nil {
		return "", err
	}

	//Checking for sufficient balance
	if balance < amount {
		return "", fmt.Errorf("insufficient balance")
	}

	//Loop through UTXO to find candidates
	var amountIn int64 = 0
	var pkScripts [][]byte
	for _, v := range *utxos {

		utxoHash, err := chainhash.NewHashFromStr(v.Txid)
		if err != nil {
			fmt.Println("could not get hash from transaction ID; error:", err)
			return "", err
		}

		outPoint := wire.NewOutPoint(utxoHash, uint32(v.Vout))
		txIn := wire.NewTxIn(outPoint, nil, nil)

		floatAmount := v.Amount.Mul(decimal.NewFromFloat(1e8))
		utxoAmount, exact := floatAmount.Float64()
		if exact != true {
			fmt.Println("could not convert utxoAmount from decimal to float precisely; err:", err)
			return "", err
		}

		amountIn += int64(utxoAmount)

		//Append ScriptPubKey to the list of scripts
		utxoPkScript, err := hex.DecodeString(v.ScriptPubKey)
		if err != nil {
			return "", err
		}
		pkScripts = append(pkScripts, utxoPkScript)

		//Append Transaction
		redeemTx.AddTxIn(txIn)

		//Once we gathered all the UTXOs we need, we stop
		if amountIn >= amount {
			break
		}

	}

	//Get destination address as []byte from function argument (destination)
	destinationAddr, err := qtumsuite.DecodeAddress(destination, &qtumTestNetParams)
	if err != nil {
		return "", err
	}

	//Generate PayToAddressScript
	destinationScript, err := txscript.PayToAddrScript(destinationAddr)
	if err != nil {
		return "", err
	}

	/*
		ADD OP CODES FOR CONTRACT CREATION TO THE TX OUTPUT

	*/

	//Adding the destination address and the amount to the transaction as output
	redeemTxOut := wire.NewTxOut(amount, destinationScript)
	redeemTx.AddTxOut(redeemTxOut)

	//Need to look into the actual fee
	var change int64 = amountIn - amount - 100000

	//Get address
	addrPubKey, err := qtumsuite.NewAddressPubKey(wif.SerializePubKey(), &chaincfg.TestNet3Params)

	//Generate PayToAddrScript for source address
	changeScript, err := txscript.PayToAddrScript(addrPubKey)
	if err != nil {
		return "", err
	}

	chanceTxOut := wire.NewTxOut(change, changeScript)
	redeemTx.AddTxOut(chanceTxOut)

	// Sign the Tx
	finalRawTx, err := SignTx(redeemTx, pkScripts, wif)

	return finalRawTx, nil
}

func CreateContractTx(privKey string, destination string, amount int64, data string) (string, error) {

	var qtumTestNetParams = chaincfg.MainNetParams
	//TestnetParams
	qtumTestNetParams.PubKeyHashAddrID = 120
	qtumTestNetParams.ScriptHashAddrID = 110

	redeemTx := wire.NewMsgTx(wire.TxVersion)

	//Decode WIF
	wif, err := qtumsuite.DecodeWIF(privKey)
	if err != nil {
		return "", err
	}

	//Gather info extracted from UTXOs related to addrPubKey (prevTxId, balance, pkScript)
	utxos, balance, err := GatherUTXOs(wif.SerializePubKey(), redeemTx)
	if err != nil {
		return "", err
	}

	//Checking for sufficient balance
	if balance < amount {
		return "", fmt.Errorf("insufficient balance")
	}

	//Loop through UTXO to find candidates
	var amountIn int64 = 0
	var pkScripts [][]byte
	for _, v := range *utxos {

		utxoHash, err := chainhash.NewHashFromStr(v.Txid)
		if err != nil {
			fmt.Println("could not get hash from transaction ID; error:", err)
			return "", err
		}

		outPoint := wire.NewOutPoint(utxoHash, uint32(v.Vout))
		txIn := wire.NewTxIn(outPoint, nil, nil)

		floatAmount := v.Amount.Mul(decimal.NewFromFloat(1e8))
		utxoAmount, exact := floatAmount.Float64()
		if exact != true {
			fmt.Println("could not convert utxoAmount from decimal to float precisely; err:", err)
			return "", err
		}

		amountIn += int64(utxoAmount)

		//Append ScriptPubKey to the list of scripts
		utxoPkScript, err := hex.DecodeString(v.ScriptPubKey)
		if err != nil {
			return "", err
		}
		pkScripts = append(pkScripts, utxoPkScript)

		//Append Transaction
		redeemTx.AddTxIn(txIn)

		//Once we gathered all the UTXOs we need, we stop
		if amountIn >= amount {
			break
		}

	}

	//Get destination address as []byte from function argument (destination)

	//Need to look into the actual fee
	var change int64 = amountIn - amount - 100000

	//Get address
	addrPubKey, err := qtumsuite.NewAddressPubKey(wif.SerializePubKey(), &chaincfg.TestNet3Params)

	//Generate PayToAddrScript for source address
	changeScript, err := txscript.PayToAddrScript(addrPubKey)
	if err != nil {
		return "", err
	}

	chanceTxOut := wire.NewTxOut(change, changeScript)

	/*

		destinationAddr, err := qtumsuite.DecodeAddress(destination, &qtumTestNetParams)
		if err != nil {
			return "", err
		}

		//Generate PayToAddressScript
		destinationScript, err := txscript.PayToAddrScript(destinationAddr)
		if err != nil {
			return "", err
		}
			ADD OP CODES FOR CONTRACT CREATION TO THE TX OUTPUT
			// 1    // address type of the pubkeyhash (public key hash)
			// Address               // sender's pubkeyhash address
			// {signature, pubkey}   //serialized scriptSig
			// OP_SENDER
			// 4                     // EVM version
			// 100000                //gas limit
			// 10                    //gas price
			// 1234                  // data to be sent by the contract
			// OP_CREATE

			data is

			608060405234801561001057600080fd5b5060c78061001f6000396000f3fe6080604052348015600f57600080fd5b506004361060325760003560e01c806360fe47b11460375780636d4ce63c146062575b600080fd5b606060048036036020811015604b57600080fd5b8101908080359060200190929190505050607e565b005b60686088565b6040518082815260200191505060405180910390f35b8060008190555050565b6000805490509056fea264697066735822122083c1f201c2ec2cd8a9fa8c8e2ec8d37fd84917c7fcb9fb4ddf93cf2e55ac297064736f6c63430007040033

	*/
	contractScript, err := CreateContractScript(data, addrPubKey)

	//Adding the destination address and the amount to the transaction as output
	redeemTxOut := wire.NewTxOut(amount, contractScript)
	redeemTx.AddTxOut(redeemTxOut)

	//Add change to tx out
	redeemTx.AddTxOut(chanceTxOut)

	// Sign the Tx
	finalRawTx, err := SignTx(redeemTx, pkScripts, wif)

	return finalRawTx, nil
}

func CreateContractScript(data string, addrPubKey *qtumsuite.AddressPubKey) ([]byte, error) {
	/*
		ADD OP CODES FOR CONTRACT CREATION TO THE TX OUTPUT
			// 1    // address type of the pubkeyhash (public key hash)
			// Address               // sender's pubkeyhash address
			// {signature, pubkey}   //serialized scriptSig
			// OP_SENDER
			// 4                     // EVM version
			// 100000                //gas limit
			// 10                    //gas price
			// 1234                  // data to be sent by the contract
			// OP_CREATE

			Might have to worry about the "type"
	*/
	scriptBuilder := txscript.NewScriptBuilder().AddData([]byte{1})
	scriptBuilder.AddData(addrPubKey.AddressPubKeyHash().ScriptAddress())

	return []byte{0}, nil
}

func SignTx(redeemTx *wire.MsgTx, sourcePkScript [][]byte, wif *qtumsuite.WIF) (string, error) {

	for i := range redeemTx.TxIn {

		//Generate signature script

		signatureHash, err := txscript.CalcSignatureHash(sourcePkScript[i], txscript.SigHashAll, redeemTx, i)
		if err != nil {
			return "", err
		}

		/*
			Sometimes the signing process doesn't work
		*/
		privKey := secp256k1.PrivKeyFromBytes(wif.PrivKey.Serialize())

		signature := ecdsa.Sign(privKey, signatureHash)

		signatureScript, err := txscript.NewScriptBuilder().AddData(append(signature.Serialize(), byte(txscript.SigHashAll))).Script()
		if err != nil {
			return "", err
		}

		redeemTx.TxIn[i].SignatureScript = signatureScript
	}

	buf := bytes.NewBuffer(make([]byte, 0, redeemTx.SerializeSize()))
	redeemTx.Serialize(buf)

	hexSignedTx := hex.EncodeToString(buf.Bytes())

	return hexSignedTx, nil
}
