package main

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"math/big"
	"net/http"
	"os"
	"strconv"
	"time"
)

var (
	logger             *zap.Logger
	cfg                *viper.Viper
	integerDefault     int
	privateKeyDefault  string
	portDefault        string
	accounts           = make(map[string]Account)
	explorerUrlDefault string
)

type RequestBody struct {
	Network string `json:"network" banding:"required"`
	Address string `json:"address" banding:"required"`
	Amount  string `json:"amount" banding:"required"`
}
type ApiResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"` // 使用 interface{} 类型允许这个字段保存任何类型的数据
}

// 记录账户领取的时间
type Account struct {
	Address          string    `json:"address"`
	LastWithdrawTime time.Time `json:"last_withdraw_time"`
}

func main() {
	// 初始化日志记录器
	initLogger()
	defer logger.Sync()

	// 初始化配置管理器
	var err error
	cfg, err = initConfig()
	if err != nil {
		logger.Error("Failed to initialize config", zap.Error(err))
		os.Exit(1)
	}
	// 创建 Gin 引擎
	r := gin.Default()

	// 设置路由
	r.POST("/mantle/request", handleWithdraw)

	r.Run(portDefault)
}

func handleWithdraw(c *gin.Context) {
	var req RequestBody
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ApiResponse{
			Success: false,
			Message: err.Error(),
		})
		return
	}

	Address := req.Address
	if !common.IsHexAddress(Address) {
		c.JSON(http.StatusBadRequest, ApiResponse{
			Success: false,
			Message: "Please check and enter a valid wallet address.",
		})
		return
	}

	account_user, exists := accounts[Address]
	if exists {
		// 检查是否满足 24 小时的条件
		duration := time.Duration(integerDefault) * time.Hour
		if time.Since(account_user.LastWithdrawTime) < duration {
			c.JSON(http.StatusForbidden, ApiResponse{
				Success: false,
				Message: "You can only withdraw once every 24 hours.",
			})
			return
		}
	}

	// 金额转换 wei=>eth
	amountFloat64, err := strconv.ParseFloat(req.Amount, 64)
	if err != nil {
		fmt.Println("Error converting string to float64:", err)
		return
	}
	amount := big.NewFloat(amountFloat64)
	amount = amount.Mul(amount, big.NewFloat(1e18))
	intAmount := new(big.Int)
	amount.Int(intAmount)

	client, err := ethclient.DialContext(context.Background(), "https://"+req.Network)
	if err != nil {
		logger.Error("Failed to connect to Mantle client", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ApiResponse{
			Success: false,
			Message: "Failed to connect to Mantle client",
		})
		return
	}
	defer client.Close()

	privateKey, err := crypto.HexToECDSA(privateKeyDefault)
	if err != nil {
		logger.Error("Failed to decode private key", zap.Error(err))

		c.JSON(http.StatusInternalServerError, ApiResponse{
			Success: false,
			Message: "Failed to decode private key",
		})
		return
	}

	publicKey := privateKey.Public()
	publicKeyECDSA, ok := publicKey.(*ecdsa.PublicKey)

	if !ok {
		logger.Error("Failed to decode private key", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ApiResponse{
			Success: false,
			Message: "Failed to decode private key",
		})
		return
	}

	fromAddress := crypto.PubkeyToAddress(*publicKeyECDSA)

	account := common.HexToAddress(fromAddress.String()) // vitalik
	balance, _ := client.BalanceAt(context.Background(), account, nil)
	if balance.Cmp(intAmount) == -1 {
		logger.Error("Insufficient balance", zap.Error(err))
		c.JSON(http.StatusBadRequest, ApiResponse{
			Success: false,
			Message: "Insufficient balance",
		})
		return
	}

	nonce, err := client.PendingNonceAt(context.Background(), fromAddress)
	if err != nil {
		logger.Error("Failed to get nonce", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ApiResponse{
			Success: false,
			Message: "Failed to get nonce",
		})
		return
	}

	gasFeeCap, err := client.SuggestGasPrice(context.Background())
	if err != nil {
		logger.Error("Failed to get gas price", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ApiResponse{
			Success: false,
			Message: "Failed to get gas price",
		})
		return
	}

	gasTipCap, err := client.SuggestGasTipCap(context.Background())
	if err != nil {
		logger.Error("Failed to get gas tip cap", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ApiResponse{
			Success: false,
			Message: "Failed to get gas tip cap",
		})
		return

	}
	var data []byte

	chainID, err := client.NetworkID(context.Background())
	if err != nil {
		logger.Error("Failed to get network ID", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ApiResponse{
			Success: false,
			Message: "Failed to get network ID",
		})
		return
	}

	toAddress := common.HexToAddress(Address)
	//gasLimit := uint64(21000)

	// 动态估算 gasLimit
	msg := ethereum.CallMsg{
		From:      fromAddress,
		To:        &toAddress,
		GasFeeCap: gasFeeCap,
		GasTipCap: gasTipCap,
		Value:     intAmount,
		Data:      data,
	}
	gasLimit, err := client.EstimateGas(context.Background(), msg)
	if err != nil {
		logger.Error("Failed to estimate gas", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ApiResponse{
			Success: false,
			Message: "Failed to estimate gas",
		})
		return
	}

	// 构造交易
	tx := types.NewTx(&types.DynamicFeeTx{
		ChainID:   chainID,
		Nonce:     nonce,
		GasFeeCap: gasFeeCap,
		GasTipCap: gasTipCap,
		Gas:       gasLimit,
		To:        &toAddress,
		Value:     intAmount,
		Data:      data,
	})

	// 发送交易
	signedTx, err := types.SignTx(tx, types.NewLondonSigner(chainID), privateKey)
	err = client.SendTransaction(context.Background(), signedTx)
	if err != nil {
		logger.Error(err.Error())
		c.JSON(http.StatusInternalServerError, ApiResponse{
			Success: false,
			Message: "Deal failed",
		})
		return
	}

	accounts[Address] = Account{
		Address:          Address,
		LastWithdrawTime: time.Now(),
	}

	c.JSON(http.StatusOK, gin.H{
		"success":      true,
		"tx_id":        signedTx.Hash().Hex(),
		"explorer_url": explorerUrlDefault + signedTx.Hash().Hex(),
	})
}

// initLogger 初始化日志记录器
func initLogger() {
	var err error
	logger, err = zap.NewProduction()
	if err != nil {
		fmt.Println("Failed to initialize logger:", err)
		os.Exit(1)
	}
}

// initConfig 初始化配置管理器
func initConfig() (*viper.Viper, error) {
	v := viper.New()
	v.SetConfigName("configs")
	v.SetConfigType("yaml")
	v.AddConfigPath("./")
	err := v.ReadInConfig()
	if err != nil {
		return nil, err
	}
	logger.Info("Config initialized successfully")

	// 获取配置文件中的配置信息
	integerDefault = v.GetInt("interval")
	privateKeyDefault = v.GetString("privateKey")
	portDefault = fmt.Sprintf(":%d", v.GetInt("port"))
	explorerUrlDefault = v.GetString("sepolia.explorerUrl")
	return v, nil
}
