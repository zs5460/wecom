# WeCom SDK


本项目基于github.com/esap/wechat精简，专用于企业微信收发文本消息。

## 快速开始


```go
package main

import (
	"net/http"

	"github.com/zs5460/wecom" // 微信SDK包
)

func main() {
	
	cfg := &wecom.WxConfig{
		Token:          "yourToken",
		AppId:          "yourAppID",
		AgentId:        "yourAgentId",
		Secret:         "yourSecret",
		EncodingAESKey: "yourEncodingAesKey",
	}

	app := wecom.New(cfg)

    // 主动推送消息
	app.SendText("@all", "Hello,World!")
	
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// 回复消息
		ctx := app.VerifyURL(w, r)
		ctx.NewText("消息已收到：" + ctx.Msg.Content + ",处理中...").Reply()
	})
	
	http.ListenAndServe(":8080", nil)
}

```

## License

MIT
