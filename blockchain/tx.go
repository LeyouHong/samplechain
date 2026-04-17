package blockchain

// TXOutput 代表一笔交易的输出，即一个未花费的 UTXO
// Value  : 该输出锁定的代币数量
// PubKey : 能够解锁该输出的公钥（此处简化为地址字符串）
type TXOutput struct {
	Value  int
	PubKey string
}

// TXInput 代表一笔交易的输入，指向某个之前交易的输出（UTXO）
// ID  : 被引用的前序交易 ID
// Out : 被引用输出在前序交易 Outputs 切片中的索引
// Sig : 解锁脚本（此处简化为发送方地址，用于证明有权花费该 UTXO）
type TXInput struct {
	ID  []byte
	Out int
	Sig string
}

// CanUnlock 判断该输入是否由 data（地址）持有者创建
// 即验证发送方是否有权花费这笔 UTXO（简化版签名验证）
func (in *TXInput) CanUnlock(data string) bool {
	return in.Sig == data
}

// CanBeUnlocked 判断该输出是否可被 data（地址）持有者解锁花费
// 即验证接收方地址是否匹配（简化版锁定脚本验证）
func (out *TXOutput) CanBeUnlocked(data string) bool {
	return out.PubKey == data
}
