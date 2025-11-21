# audiorelay-beta

基于portaudio实现音频网络转发(TCP/HTTP)的Go服务端

--

### 可能的预先准备

⚠️：仅在macos13.7.4上通过

因macos不支持内录系统音频，您需要安装[BlackHole](https://github.com/ExistentialAudio/BlackHole) （audiorelay会自动选择BlackHole作为捕获输入源）

若您的系统没有portaudio依赖导致运行异常您可能需要

```bash
brew install portaudio
```

### 启动示例

```go
package main

import (
	"audiorelay/audiorelay"
	"fmt"
)

func main() {
	if err := audiorelay.StartWithConfig("config.yml"); err != nil {
		fmt.Println(err)
	}
}
```

### 目录结构

```
audiorelay/
├── main.go                 # 程序入口
├── config.yaml            # 配置文件
├── go.mod                 # Go 模块列表定义
├── audiorelay/            
│   ├── relay.go           # 主服务逻辑
│   ├── config.go          # 配置管理
│   ├── audio.go           # 音频捕获和处理
│   ├── tcp.go             # TCP 服务器
│   └── http.go            # HTTP 服务器和 Web 界面
└── web/
    └── index.html         # Web 访问页面
```

### 帧状态:

```
Audio Status: Streaming | Frames: 4719 | Buffer: 2048 | Total: 18.1 MB | Rate: 187.5 KB/s | Silence: 21.5%
```

| 指标        | 说明                 | 示例值             |
| :---------- | :------------------- | :----------------- |
| **Status**  | 区别间隔无声帧       | Streamning/ Silent |
| **Frames**  | 处理的音频帧总数     | 4719               |
| **Buffer**  | 实际使用的缓冲区大小 | 2048               |
| **Total**   | 累计传输数据量       | 18.1 MB            |
| **Rate**    | 当前传输速率         | 187.5 KB/s         |
| **Silence** | 静音检测百分比       | 21.5%              |