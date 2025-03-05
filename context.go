package wecom

import (
	"encoding/xml"
	"errors"
	"net/http"
	"strings"
	"time"
)

// context 消息上下文
type context struct {
	server    *Server
	timestamp string
	nonce     string
	Msg       *WxMsg
	Resp      any
	Writer    http.ResponseWriter
	Request   *http.Request
	hasReply  bool
}

// Reply 被动回复消息
func (c *context) Reply() (err error) {
	if c.hasReply {
		return errors.New("重复调用错误")
	}

	c.hasReply = true

	if c.Resp == nil {
		return nil
	}

	printf("[wecom] Reply Wechat <== %+v\n", c.Resp)
	if c.server.safeMode {
		b, err := xml.MarshalIndent(c.Resp, "", "  ")
		if err != nil {
			return err
		}
		printf("[wecom] Reply Wechat xml.Marshal result:\n%s\n", string(b))
		c.Resp, err = c.server.encryptMsg(b, c.timestamp, c.nonce)
		if err != nil {
			return err
		}
	}
	c.Writer.Header().Set("Content-Type", "application/xml;charset=UTF-8")
	return xml.NewEncoder(c.Writer).Encode(c.Resp)
}

// Send 主动发送消息(客服)
func (c *context) Send() *context {
	c.server.AddMsg(c.Resp)
	return c
}

func (c *context) newResp(msgType string) wxResp {
	return wxResp{
		FromUserName: cdata(c.Msg.ToUserName),
		ToUserName:   cdata(c.Msg.FromUserName),
		MsgType:      cdata(msgType),
		CreateTime:   time.Now().Unix(),
		AgentId:      c.Msg.AgentID,
		Safe:         c.server.safe,
	}
}

// NewText 新建文本消息
func (c *context) NewText(text ...string) *context {
	c.Resp = &Text{
		wxResp:  c.newResp("text"),
		content: content{cdata(strings.Join(text, ""))}}
	return c
}
