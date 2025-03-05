package wecom

import (
	"fmt"
	"time"
)

type accessToken struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int64  `json:"expires_in"`
	WxErr
}

// getAccessToken 读取AccessToken
func (s *server) getAccessToken() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var err error
	if s.accessToken == nil || s.accessToken.ExpiresIn < time.Now().Unix() {
		for range 3 {
			err = s.getAccessTokenFromNet()
			if err == nil {
				break
			}
			printf("[wecom] getAccessTokenFromNet[%v] %v", s.agentId, err)
			time.Sleep(time.Second)
		}
		if err != nil {
			return ""
		}
	}
	return s.accessToken.AccessToken
}

func (s *server) getAccessTokenFromNet() (err error) {
	url := fmt.Sprintf(s.tokenUrl, s.appId, s.secret)
	at := new(accessToken)
	if err = getJson(url, at); err != nil {
		return
	}
	if at.ErrCode > 0 {
		return at.Error()
	}
	at.ExpiresIn = time.Now().Unix() + at.ExpiresIn - 5
	s.accessToken = at
	printf("[wecom] getAccessTokenFromNet:%v", s.accessToken)
	return

}
