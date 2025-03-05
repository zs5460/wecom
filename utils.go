package wecom

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	mr "math/rand"
)

// aesDecrypt AES-CBC解密,PKCS#7,传入密文和密钥，[]byte
func aesDecrypt(src, key []byte) (dst []byte, err error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	iv := make([]byte, aes.BlockSize)
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return nil, err
	}
	dst = make([]byte, len(src))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(dst, src)

	return pkcs7UnPad(dst), nil
}

// pkcs7UnPad PKSC#7解包
func pkcs7UnPad(msg []byte) []byte {
	length := len(msg)
	padlen := int(msg[length-1])
	return msg[:length-padlen]
}

// aesEncrypt AES-CBC加密+PKCS#7打包，传入明文和密钥
func aesEncrypt(src []byte, key []byte) ([]byte, error) {
	k := len(key)
	src = pkcs7Pad(src, k)

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	iv := make([]byte, aes.BlockSize)
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return nil, err
	}

	dst := make([]byte, len(src))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(dst, src)

	return dst, nil
}

// pkcs7Pad PKCS#7打包
func pkcs7Pad(msg []byte, blockSize int) []byte {
	if blockSize < 1<<1 || blockSize >= 1<<8 {
		panic("unsupported block size")
	}
	padlen := blockSize - len(msg)%blockSize
	padding := bytes.Repeat([]byte{byte(padlen)}, padlen)
	return append(msg, padding...)
}

// sortSha1 排序并sha1，主要用于计算signature
func sortSha1(s ...string) string {
	sort.Strings(s)
	h := sha1.New()
	h.Write([]byte(strings.Join(s, "")))
	return fmt.Sprintf("%x", h.Sum(nil))
}

// timeOut 全局请求超时设置,默认1分钟
var timeOut time.Duration = 60 * time.Second

// SetTimeOut 设置全局请求超时
func SetTimeOut(d time.Duration) {
	timeOut = d
}

// httpClient() 带超时的http.Client
func httpClient() *http.Client {
	return &http.Client{Timeout: timeOut}
}

// getJson 发送GET请求解析json
func getJson(uri string, v any) error {

	r, err := httpClient().Get(uri)
	if err != nil {
		return err
	}
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(v)
}

// postJson 发送Json格式的POST请求
func postJson(uri string, obj interface{}) ([]byte, error) {
	buf := new(bytes.Buffer)
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	err := enc.Encode(obj)
	if err != nil {
		return nil, err
	}
	resp, err := httpClient().Post(uri, "application/json;charset=utf-8", buf)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http post error : uri=%v , statusCode=%v", uri, resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// getRandomString 获得随机字符串
func getRandomString(l int) string {
	str := "0123456789abcdefghijklmnopqrstuvwxyz"
	bytes := []byte(str)
	result := []byte{}
	r := mr.New(mr.NewSource(time.Now().UnixNano()))

	for i := 0; i < l; i++ {
		result = append(result, bytes[r.Intn(len(bytes))])
	}
	return string(result)
}

// substr 截取字符串 start 起点下标 end 终点下标(不包括)
func substr(str string, start int, end int) string {
	rs := []rune(str)
	length := len(rs)

	if start < 0 || start > length || end < 0 {
		return ""
	}

	if end > length {
		return string(rs[start:])
	}
	return string(rs[start:end])
}
