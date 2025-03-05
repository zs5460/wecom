package wecom

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sync"
	"unicode/utf8"
)

// corpAPI 企业微信接口，相关接口常量统一以此开头
const (
	corpAPI      = "https://qyapi.weixin.qq.com/cgi-bin/"
	corpAPIToken = corpAPI + "gettoken?corpid=%s&corpsecret=%s"
	corpAPIMsg   = corpAPI + "message/send?access_token="
)

const (
	dataFormatXML  = "XML" // default format
	dataFormatJSON = "JSON"
)

var (
	// 调试模式，开启后会打印调试信息
	Debug bool = false
)

// WxConfig 配置，用于New()
type WxConfig struct {
	AppId          string
	Token          string
	Secret         string
	EncodingAESKey string
	AgentId        int
	AppName        string
	DataFormat     string // 数据格式:JSON、XML
}

// server 微信服务容器
type server struct {
	appId          string
	agentId        int
	secret         string
	token          string
	encodingAESKey string

	appName    string // 唯一标识，主要用于企业微信多应用区分
	aesKey     []byte // 解密的AesKey
	safeMode   bool
	entMode    bool
	dataFormat string // 通讯数据格式:JSON、XML

	rootUrl  string
	msgUrl   string
	tokenUrl string

	safe        int
	accessToken *accessToken

	msgQueue chan any
	mu       sync.Mutex // accessToken读取锁

}

// New 新建微信服务容器
func New(wc *WxConfig) *server {
	s := &server{
		appId:          wc.AppId,
		secret:         wc.Secret,
		agentId:        wc.AgentId,
		appName:        wc.AppName,
		token:          wc.Token,
		encodingAESKey: wc.EncodingAESKey,
		dataFormat:     wc.DataFormat,
	}

	// Set XML as default when data format is no setting.
	if s.dataFormat == "" {
		s.dataFormat = dataFormatXML
	}

	s.rootUrl = corpAPI
	s.msgUrl = corpAPIMsg
	s.tokenUrl = corpAPIToken
	s.entMode = true

	err := s.getAccessTokenFromNet()
	if err != nil {
		printf("[wecom] getAccessToken err:%+v\n", err)
	}

	// 存在EncodingAESKey则开启加密安全模式
	if len(s.encodingAESKey) > 0 && s.encodingAESKey != "" {
		s.safeMode = true
		if s.aesKey, err = base64.StdEncoding.DecodeString(s.encodingAESKey + "="); err != nil {
			printf("[wecom] AesKey解析错误:%+v\n", err)
		}
		println("[wecom] 启用加密模式")
	}

	s.msgQueue = make(chan any, 1000)
	go func() {
		for {
			msg := <-s.msgQueue
			e := s.SendMsg(msg)
			if e.ErrCode != 0 {
				printf("[wecom] MsgSend err:%+v\n", e.ErrMsg)
			}
		}
	}()

	return s
}

// 依据交互数据类型，从请求体中解析消息体
func (s *server) decodeMsgFromRequest(r *http.Request, msg any) error {
	if s.dataFormat == dataFormatXML {
		return xml.NewDecoder(r.Body).Decode(msg)
	} else if s.dataFormat == dataFormatJSON {
		return json.NewDecoder(r.Body).Decode(msg)
	} else {
		panic(fmt.Errorf("invalid DataFormat:%s", s.dataFormat))
	}
}

// 依据交互数据类型，从字符串中解析消息体
func (s *server) decodeMsgFromString(str string, msg any) error {
	if s.dataFormat == dataFormatXML {
		return xml.Unmarshal([]byte(str), msg)
	} else if s.dataFormat == dataFormatJSON {
		return json.Unmarshal([]byte(str), msg)
	} else {
		panic(fmt.Errorf("invalid DataFormat:%s", s.dataFormat))
	}
}

// VerifyURL 验证URL,验证成功则返回标准请求载体（Msg已解密）
func (s *server) VerifyURL(w http.ResponseWriter, r *http.Request) (ctx *context) {
	println(r.Method, "|", r.URL.String())
	ctx = &context{
		server:    s,
		Writer:    w,
		Request:   r,
		timestamp: r.FormValue("timestamp"),
		nonce:     r.FormValue("nonce"),
		Msg:       new(WxMsg),
	}

	// 明文模式可直接解析body->消息
	if !s.safeMode && r.Method == "POST" {
		if err := s.decodeMsgFromRequest(r, ctx.Msg); err != nil {
			printf("[wecom] VerifyURL Decode WxMsg err:%+v\n", err)
		}
	}

	// 密文模式，消息在body.Encrypt
	echostr := r.FormValue("echostr")
	if s.safeMode && r.Method == "POST" {
		msgEnc := new(WxMsgEnc)
		if err := s.decodeMsgFromRequest(r, msgEnc); err != nil {
			printf("[wecom] VerifyURL Decode MsgEnc err:%+v\n", err)
		}
		echostr = msgEnc.Encrypt
	}

	// 验证signature
	signature := r.FormValue("signature")
	if signature == "" {
		signature = r.FormValue("msg_signature")
	}
	if s.entMode && signature != sortSha1(s.token, ctx.timestamp, ctx.nonce, echostr) {
		println("[wecom] VerifyURL Signature验证错误!", s.token, ctx.timestamp, ctx.nonce, echostr)
		return
	}

	// 密文模式，解密echostr中的消息
	if s.entMode || (s.safeMode && r.Method == "POST") {
		var err error
		echostr, err = s.decryptMsg(echostr)
		if err != nil {
			printf("[wecom] VerifyURL DecryptMsg error:%+v\n", err)
			return
		}
	}

	if r.Method == "GET" {
		printf("[wecom] VerifyURL api echostr:%s\n", echostr)
		w.Write([]byte(echostr))
		return
	}

	if s.safeMode {
		if err := s.decodeMsgFromString(echostr, ctx.Msg); err != nil {
			printf("[wecom] VerifyURL Msg parse err:%+v\n", err)
		}
	}
	printf("[wecom] VerifyURL parsed Msg:%+v\n", ctx.Msg)
	return
}

// decryptMsg 解密微信消息,密文string->base64Dec->aesDec->去除头部随机字串
// AES加密的buf由16个字节的随机字符串、4个字节的msg_len(网络字节序)、msg和$AppId组成
func (s *server) decryptMsg(msg string) (string, error) {
	aesMsg, err := base64.StdEncoding.DecodeString(msg)
	if err != nil {
		return "", err
	}

	buf, err := aesDecrypt(aesMsg, s.aesKey)
	if err != nil {
		return "", err
	}

	var msgLen int32
	binary.Read(bytes.NewBuffer(buf[16:20]), binary.BigEndian, &msgLen)
	if msgLen < 0 || msgLen > 1000000 {
		return "", errors.New("AesKey is invalid")
	}
	if string(buf[20+msgLen:]) != s.appId {
		return "", errors.New("AppId is invalid")
	}
	return string(buf[20 : 20+msgLen]), nil
}

// wxRespEnc 加密回复体
type wxRespEnc struct {
	XMLName      xml.Name `xml:"xml"`
	Encrypt      cdata
	MsgSignature cdata
	TimeStamp    string
	Nonce        cdata
}

// encryptMsg 加密普通回复(AES-CBC),打包成xml格式
// AES加密的buf由16个字节的随机字符串、4个字节的msg_len(网络字节序)、msg和$AppId组成
func (s *server) encryptMsg(msg []byte, timeStamp, nonce string) (re *wxRespEnc, err error) {
	buf := new(bytes.Buffer)
	err = binary.Write(buf, binary.BigEndian, int32(len(msg)))
	if err != nil {
		return
	}
	l := buf.Bytes()

	rd := []byte(getRandomString(16))

	plain := bytes.Join([][]byte{rd, l, msg, []byte(s.appId)}, nil)
	ae, _ := aesEncrypt(plain, s.aesKey)
	encMsg := base64.StdEncoding.EncodeToString(ae)
	re = &wxRespEnc{
		Encrypt:      cdata(encMsg),
		MsgSignature: cdata(sortSha1(s.token, timeStamp, nonce, encMsg)),
		TimeStamp:    timeStamp,
		Nonce:        cdata(nonce),
	}
	return
}

// AddMsg 添加队列消息
func (s *server) AddMsg(v any) {
	s.msgQueue <- v
}

// SendMsg 发送消息
func (s *server) SendMsg(v any) *WxErr {
	url := s.msgUrl + s.getAccessToken()
	body, err := postJson(url, v)
	if err != nil {
		return &WxErr{-1, err.Error()}
	}
	rst := new(WxErr)
	err = json.Unmarshal(body, rst)
	if err != nil {
		return &WxErr{-1, err.Error()}
	}
	printf("[wecom] SendMsg 发送消息:%+v\n 回执:%+v\n", v, *rst)
	return rst
}

// SendText 发送文本消息,过长时按500长度自动拆分
func (s *server) SendText(to, msg string) *WxErr {
	const maxMsgLength = 500
	length := utf8.RuneCountInString(msg)
	parts := length/maxMsgLength + 1

	if parts == 1 {
		return s.SendMsg(s.NewText(to, msg))
	}
	for i := 0; i < parts; i++ {
		partMsg := fmt.Sprintf("%s\n(%d/%d)", substr(msg, i*maxMsgLength, (i+1)*maxMsgLength), i+1, parts)
		err := s.SendMsg(s.NewText(to, partMsg))
		if err.ErrCode != 0 {
			return err
		}
	}
	return nil
}

// println Debug输出
func println(v ...any) {
	if Debug {
		log.Println(v...)
	}
}

// printf Debug输出
func printf(s string, v ...any) {
	if Debug {
		log.Printf(s, v...)
	}
}
