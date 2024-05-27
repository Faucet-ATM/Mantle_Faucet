# Mantle_Faucet
***
这是一个基于 Go 语言和 Gin 框架编写的Mantle_Faucet。
## 使用方法
1. clone
```bash
    git clone https://github.com/StareMe/Mantle_Faucet.git
```
2. 进入项目目录
```bash
    cd Mantle_Faucet
```
3. 安装依赖
```bash
    go mod tidy
```
4. 添加配置信息
```yaml
   port: 6010
   interval: 24 # (24 hours) collection interval
   privateKey: Your wallet's private key 
   sepolia:
     explorerUrl: https://explorer.sepolia.mantle.xyz/tx/ 
```
5. 运行
```bash
    go run main.go
```