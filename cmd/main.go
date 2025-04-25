package main

import (
	"context"
	"fmt"

	"github.com/go-deepseek/deepseek"
	"github.com/go-deepseek/deepseek/request"
)

func main() {
	client, _ := deepseek.NewClient("sk-3b423cb5e6774f8eb5234a7a40af09cb")

	chatReq := &request.ChatCompletionsRequest{
		Model:  deepseek.DEEPSEEK_CHAT_MODEL,
		Stream: false,
		Messages: []*request.Message{
			{
				Role:    "user",
				Content: "Hello Deepseek!", // set your input message
			},
		},
	}

	chatResp, err := client.CallChatCompletionsChat(context.Background(), chatReq)
	if err != nil {
		fmt.Println("Error =>", err)
		return
	}
	fmt.Printf("output => %s\n", chatResp.Choices[0].Message.Content)
}
