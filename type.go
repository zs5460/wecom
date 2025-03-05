package wecom

import (
	"encoding/xml"
	"fmt"
	"strings"
)

// WxErr 通用错误
type WxErr struct {
	ErrCode int
	ErrMsg  string
}

// Error 返回错误信息，如果没有错误则返回 nil
func (w *WxErr) Error() error {
	if w.ErrCode != 0 {
		return fmt.Errorf("err: errcode=%v , errmsg=%v", w.ErrCode, w.ErrMsg)
	}
	return nil
}

// cdata 标准规范，XML编码成 `<![cdata[消息内容]]>`
type cdata string

// MarshalXML 自定义 XML 编码接口，实现将字符串编码为 CDATA 格式
func (c cdata) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	return e.EncodeElement(struct {
		string `xml:",cdata"`
	}{string(c)}, start)
}

// wxResp 响应消息共用字段
// 响应消息被动回复为XML结构，文本类型采用CDATA编码规范
// 响应消息主动发送为json结构，即客服消息
type wxResp struct {
	XMLName      xml.Name `xml:"xml" json:"-"`
	ToUserName   cdata    `json:"touser"`
	ToParty      cdata    `xml:"-" json:"toparty"` // 企业号专用
	ToTag        cdata    `xml:"-" json:"totag"`   // 企业号专用
	FromUserName cdata    `json:"-"`
	CreateTime   int64    `json:"-"`
	MsgType      cdata    `json:"msgtype"`
	AgentId      int      `xml:"-" json:"agentid"`
	Safe         int      `xml:"-" json:"safe"`
}

// to字段格式："userid1|userid2 deptid1|deptid2 tagid1|tagid2"
func (s *server) newWxResp(msgType, to string) (r wxResp) {
	toArr := strings.Split(to, " ")
	r = wxResp{
		ToUserName: cdata(toArr[0]),
		MsgType:    cdata(msgType),
		AgentId:    s.agentId,
		Safe:       s.safe}
	if len(toArr) > 1 {
		r.ToParty = cdata(toArr[1])
	}
	if len(toArr) > 2 {
		r.ToTag = cdata(toArr[2])
	}
	return
}

// Text 文本消息
type (
	Text struct {
		wxResp
		content `xml:"Content" json:"text"`
	}

	content struct {
		Content cdata `json:"content"`
	}
)

// NewText Text 文本消息
func (s *server) NewText(to string, msg ...string) Text {
	txt := Text{
		s.newWxResp("text", to),
		content{cdata(strings.Join(msg, ""))},
	}
	printf("[wecom] NewText:%+v", txt)
	return txt
}

type (
	// WxMsg 混合用户消息，业务判断的主体
	WxMsg struct {
		XMLName      xml.Name `xml:"xml"`
		ToUserName   string
		FromUserName string
		CreateTime   int64
		MsgId        int64
		MsgType      string
		Content      string // text
		AgentID      int    // corp
	}

	// WxMsgEnc 加密的用户消息
	WxMsgEnc struct {
		XMLName    xml.Name `xml:"xml"`
		ToUserName string
		AgentID    int
		Encrypt    string
		AgentType  string
	}
)
