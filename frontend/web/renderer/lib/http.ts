import axios, { type AxiosInstance, type AxiosRequestConfig } from 'axios'
import { message } from './tips_utils'
import { getFetchApiKey } from './utils'

// Define API configurations
export const API_CONFIG = {
  ADMIN: 'https://api.zgi.ai',
  CLIENT: 'https://api.zgi.ai',
  COMMON: '/api',
  // COMMON: 'http://localhost:7007',
} as const

// Type for API endpoints
export type ApiEndpoint = keyof typeof API_CONFIG

interface HttpConfig extends AxiosRequestConfig {
  endpoint?: ApiEndpoint
}

class Http {
  private static instance: Http | null = null
  private instances: Map<ApiEndpoint, AxiosInstance>
  private defaultEndpoint: ApiEndpoint = 'COMMON'

  private constructor() {
    // Initialize instances map
    this.instances = new Map()
    // Create axios instance for each endpoint
    for (const endpoint of Object.keys(API_CONFIG)) {
      const axiosInstance = axios.create({
        baseURL: API_CONFIG[endpoint as ApiEndpoint],
        timeout: 10000,
        headers: {
          'Content-Type': 'application/json',
          // Add CORS headers
          'Access-Control-Allow-Origin': '*',
          'Access-Control-Allow-Methods': 'GET,PUT,POST,DELETE,PATCH,OPTIONS',
        },
      })

      // Setup interceptors for each instance
      this.setupInterceptors(axiosInstance)
      this.instances.set(endpoint as ApiEndpoint, axiosInstance)
    }
  }

  public static getInstance(): Http {
    if (this.instance === null) {
      this.instance = new Http()
    }
    return this.instance
  }

  public resetBaseURL({ endpoint, newBaseURL }: { endpoint?: ApiEndpoint; newBaseURL: string }) {
    const instance = this.instances.get(endpoint || 'COMMON')
    if (instance) {
      instance.defaults.baseURL = newBaseURL
    } else {
      throw new Error(`No instance found for endpoint: ${endpoint}`)
    }
  }

  private setupInterceptors(instance: AxiosInstance) {
    // Request interceptor
    instance.interceptors.request.use(
      (config) => {
        const token = localStorage.getItem('auth_token')
        const token_type = localStorage.getItem('token_type') || 'Bearer'
        if (config.headers && token) {
          config.headers.Authorization = `${token_type} ${token}`
        }
        return config
      },
      (error) => {
        return Promise.reject(error)
      },
    )

    // Response interceptor
    instance.interceptors.response.use(
      (response) => {
        if (response.data?.status_code > 400 && response.data?.status_message) {
          message.error(response.data?.status_message)
        }
        return response.data
      },
      (error) => {
        if (error.response) {
          switch (error.response.status) {
            case 401:
              localStorage.removeItem('auth_token')
              localStorage.removeItem('user')
              window.location.href = '/signin'
              break
            case 403:
              console.error('Forbidden access')
              break
            case 404:
              console.error('Resource not found')
              break
            case 500:
              message.error('Server error')
              break
            default:
              message.error('An error occurred')
          }
        }
        return Promise.reject(error)
      },
    )
  }

  private getInstance(endpoint?: ApiEndpoint): AxiosInstance {
    const selectedEndpoint = endpoint || this.defaultEndpoint
    const instance = this.instances.get(selectedEndpoint)
    if (!instance) {
      throw new Error(`No instance found for endpoint: ${selectedEndpoint}`)
    }
    return instance
  }

  // Generic request method
  private async request<T>(config: HttpConfig): Promise<T> {
    const { endpoint, ...axiosConfig } = config
    const instance = this.getInstance('COMMON')
    const response = await instance.request<unknown, T>(axiosConfig)
    return response
  }

  // GET method
  public async get<T>(url: string, config?: HttpConfig): Promise<T> {
    return this.request<T>({
      ...config,
      method: 'GET',
      url,
    })
  }

  // POST method
  public async post<T = any>(
    url: string,
    data?: Record<string, unknown>,
    config?: HttpConfig,
  ): Promise<T> {
    const { endpoint, ...axiosConfig } = config ?? {}
    const instance = this.getInstance('COMMON')

    if (config?.responseType === 'stream') {
      const response = await instance.post<T>(url, data, {
        ...axiosConfig,
        responseType: 'stream',
        onDownloadProgress: (progressEvent) => {
          console.debug('Download progress:', progressEvent)
        },
      })
      return response as T
    }

    return this.request<T>({
      ...config,
      method: 'POST',
      url,
      data,
    })
  }

  // PUT method
  public async put<T>(
    url: string,
    data?: Record<string, unknown>,
    config?: HttpConfig,
  ): Promise<T> {
    return this.request<T>({
      ...config,
      method: 'PUT',
      url,
      data,
    })
  }

  // DELETE method
  public async delete<T>(url: string, config?: HttpConfig): Promise<T> {
    return this.request<T>({
      ...config,
      method: 'DELETE',
      url,
    })
  }
}

// Create and export a single instance
export const http = Http.getInstance()
