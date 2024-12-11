import { http } from '@/lib/http'
import type { Role } from '@/types/chat'

interface ChatMessage {
  role: Role
  content: string
}

interface ChatCompletionOptions {
  model?: string
  temperature?: number
  presence_penalty?: number
  stream?: boolean
}

/**
 * 基于OpenAI Chat Completions API封装的请求方法
 * @param messages 消息列表
 * @param options 请求配置选项
 * @returns API响应数据
 */
export const chatCompletions = async (
  messages: ChatMessage[],
  options: ChatCompletionOptions = {},
) => {
  const { model = 'gpt-4o', temperature = 0.7, stream = false } = options

  const response = await http.post('/chat/completions', {
    model,
    messages,
    temperature,
    stream,
  })

  return response
}

/**
 * 发送消息并获取实时响应流
 * @param messages 消息列表
 * @param options 请求配置选项
 * @returns 返回一个可读流
 */
export const streamChatCompletions = async (
  messages: ChatMessage[],
  options: ChatCompletionOptions = {},
) => {
  const { model = 'gpt-3.5-turbo', temperature = 1, presence_penalty = 0 } = options

  const response = await http.post('/chat/completions', {
    model,
    messages,
    temperature,
    presence_penalty,
    stream: true,
  })

  return response
}

/**
 * 处理流式响应数据
 * @param response 响应数据流
 * @param onMessage 消息回调函数
 */
export const handleChatStream = async (response: any, onMessage: (message: string) => void) => {
  const reader = response.data // 直接使用 axios 响应中的 data

  try {
    // 创建事件源
    const eventSource = new EventSource(reader)

    return new Promise((resolve, reject) => {
      eventSource.onmessage = (event) => {
        if (event.data === '[DONE]') {
          eventSource.close()
          resolve(undefined)
          return
        }

        try {
          const data = JSON.parse(event.data)
          const content = data.choices?.[0]?.delta?.content
          if (content) {
            onMessage(content)
          }
        } catch (error) {
          console.error('解析消息失败:', error)
        }
      }

      eventSource.onerror = (error) => {
        eventSource.close()
        reject(error)
      }
    })
  } catch (error) {
    console.error('处理流失败:', error)
    throw error
  }
}
