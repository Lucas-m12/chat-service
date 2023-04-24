package chatcompletionstream

import (
	"context"
	"errors"
	"io"
	"strings"

	"github.com/lucas-m12/chat-service/internal/domain/entities"
	"github.com/lucas-m12/chat-service/internal/domain/gateway"
	openAI "github.com/sashabaranov/go-openai"
)

type ChatCompletionConfigInputDTO struct {
	Mode                 string
	ModelMaxToken        int
	Temperature          float32
	TopP                 float32
	N                    int
	Stop                 []string
	MaxTokens            int
	PresencePenalty      float32
	FrequencyPenalty     float32
	InitialSystemMessage string
}

type ChatCompletionInputDTO struct {
	ChatID      string
	UserID      string
	UserMessage string
	Config      ChatCompletionConfigInputDTO
}

type ChatCompletionOutputDTO struct {
	ChatID  string
	UserID  string
	Content string
}

type ChatCompletionUseCase struct {
	ChatGateway  gateway.ChatGateway
	OpenAIClient openAI.Client
	Stream       chan ChatCompletionOutputDTO
}

func NewChatCompletionUseCase(chatGateway gateway.ChatGateway, openAIClient openAI.Client, stream chan ChatCompletionOutputDTO) *ChatCompletionUseCase {
	return &ChatCompletionUseCase{
		ChatGateway:  chatGateway,
		OpenAIClient: openAIClient,
		Stream:       stream,
	}
}

func (useCase *ChatCompletionUseCase) Execute(ctx context.Context, input ChatCompletionInputDTO) (*ChatCompletionOutputDTO, error) {
	chat, err := useCase.ChatGateway.FindChatByID(ctx, input.ChatID)
	if err != nil {
		if err.Error() == "chat not found" {
			chat, err = CreateNewChat(input)
			if err != nil {
				return nil, errors.New("error creating new chat: " + err.Error())
			}
			err = useCase.ChatGateway.CreateChat(ctx, chat)
			if err != nil {
				return nil, errors.New("error persisting new chat: " + err.Error())
			}
		} else {
			return nil, errors.New("error fetching chat: " + err.Error())
		}
	}
	userMessage, err := entities.NewMessage("user", input.UserMessage, chat.Config.Model)
	if err != nil {
		return nil, errors.New("error creating user message: " + err.Error())
	}
	err = chat.AddMessage(userMessage)
	if err != nil {
		return nil, errors.New("error adding new message: " + err.Error())
	}
	messages := []openAI.ChatCompletionMessage{}
	for _, message := range chat.Messages {
		messages = append(messages, openAI.ChatCompletionMessage{
			Role:    message.Role,
			Content: message.Content,
		})
	}
	res, err := useCase.OpenAIClient.CreateChatCompletionStream(
		ctx,
		openAI.ChatCompletionRequest{
			Model:            chat.Config.Model.Name,
			Messages:         messages,
			MaxTokens:        chat.Config.MaxTokens,
			Temperature:      chat.Config.Temperature,
			TopP:             chat.Config.TopP,
			N:                chat.Config.N,
			Stop:             chat.Config.Stop,
			PresencePenalty:  chat.Config.PresencePenalty,
			FrequencyPenalty: chat.Config.FrequencyPenalty,
			Stream:           true,
		},
	)
	if err != nil {
		return nil, errors.New("error creating chat completion: " + err.Error())
	}
	var fullResponse strings.Builder
	for {
		response, err := res.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, errors.New("error streaming response: " + err.Error())
		}
		fullResponse.WriteString(response.Choices[0].Delta.Content)
		result := ChatCompletionOutputDTO{
			ChatID:  chat.ID,
			UserID:  chat.UserID,
			Content: fullResponse.String(),
		}
		useCase.Stream <- result
	}
	assistant, err := entities.NewMessage("assystant", fullResponse.String(), chat.Config.Model)
	if err != nil {
		return nil, errors.New("error creating assistant message: " + err.Error())
	}
	err = chat.AddMessage(assistant)
	if err != nil {
		return nil, errors.New("error adding new message: " + err.Error())
	}
	err = useCase.ChatGateway.SaveChat(ctx, chat)
	if err != nil {
		return nil, errors.New("error saving chat: " + err.Error())
	}
	return &ChatCompletionOutputDTO{
		ChatID:  chat.ID,
		UserID:  chat.UserID,
		Content: fullResponse.String(),
	}, nil
}

func CreateNewChat(input ChatCompletionInputDTO) (*entities.Chat, error) {
	model := entities.NewModel(input.Config.Mode, input.Config.MaxTokens)
	chatConfig := &entities.ChatConfig{
		Model:            model,
		Temperature:      input.Config.Temperature,
		TopP:             input.Config.TopP,
		N:                input.Config.N,
		Stop:             input.Config.Stop,
		MaxTokens:        input.Config.MaxTokens,
		PresencePenalty:  input.Config.PresencePenalty,
		FrequencyPenalty: input.Config.FrequencyPenalty,
	}
	initialMessage, err := entities.NewMessage("system", input.Config.InitialSystemMessage, model)
	if err != nil {
		return nil, errors.New("error creating initial message: " + err.Error())
	}
	chat, err := entities.NewChat(input.UserID, initialMessage, chatConfig)
	if err != nil {
		return nil, errors.New("error creating new chat: " + err.Error())
	}
	return chat, nil
}
