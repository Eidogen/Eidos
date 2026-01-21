package kafka

import (
	"crypto/sha256"
	"crypto/sha512"
	"hash"

	"github.com/xdg-go/scram"
)

// SHA256 返回 SHA256 哈希函数
var SHA256 scram.HashGeneratorFcn = func() hash.Hash { return sha256.New() }

// SHA512 返回 SHA512 哈希函数
var SHA512 scram.HashGeneratorFcn = func() hash.Hash { return sha512.New() }

// xdgScramClient 实现 sarama.SCRAMClient 接口
type xdgScramClient struct {
	*scram.Client
	*scram.ClientConversation
	HashGeneratorFcn scram.HashGeneratorFcn
}

// Begin 开始 SCRAM 认证
func (x *xdgScramClient) Begin(userName, password, authzID string) error {
	client, err := x.HashGeneratorFcn.NewClient(userName, password, authzID)
	if err != nil {
		return err
	}
	x.Client = client
	x.ClientConversation = client.NewConversation()
	return nil
}

// Step 执行 SCRAM 认证步骤
func (x *xdgScramClient) Step(challenge string) (string, error) {
	return x.ClientConversation.Step(challenge)
}

// Done 检查认证是否完成
func (x *xdgScramClient) Done() bool {
	return x.ClientConversation.Done()
}
