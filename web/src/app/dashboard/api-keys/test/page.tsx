'use client';

import React, { useState } from 'react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Textarea } from '@/components/ui/textarea';
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card';
import { Label } from '@/components/ui/label';
import { API_URL } from '@/lib/config';
import MarkdownViewer from '@/components/common/markdown-viewer';
import { Loader2, Send, Key, Bot, User } from 'lucide-react';
import { toast } from 'sonner';

export default function ApiKeyTestPage() {
  const [apiKey, setApiKey] = useState('');
  const [modelName, setModelName] = useState('gpt-4');
  const [userContent, setUserContent] = useState('你好，请介绍一下你自己。');
  const [response, setResponse] = useState('');
  const [loading, setLoading] = useState(false);
  const [usage, setUsage] = useState<any>(null);

  const handleTest = async () => {
    if (!apiKey) {
      toast.error('请输入 API Key');
      return;
    }
    if (!modelName) {
      toast.error('请输入模型名称');
      return;
    }
    if (!userContent) {
      toast.error('请输入测试内容');
      return;
    }

    setLoading(true);
    setResponse('');
    setUsage(null);

    try {
      const res = await fetch(`${API_URL}/v1/chat/completions`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${apiKey}`,
        },
        body: JSON.stringify({
          model: modelName,
          messages: [
            {
              role: 'user',
              content: userContent,
            },
          ],
          stream: false,
        }),
      });

      if (!res.ok) {
        const errorData = await res.json().catch(() => ({}));
        throw new Error(errorData.error?.message || `HTTP error! status: ${res.status}`);
      }

      const data = await res.json();
      setResponse(data.choices[0]?.message?.content || '未获取到响应内容');
      setUsage(data.usage);
      toast.success('请求成功');
    } catch (error: any) {
      console.error('Test failed:', error);
      toast.error(`测试失败: ${error.message}`);
      setResponse(`Error: ${error.message}`);
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="container max-w-4xl py-8 space-y-6">
      <div className="flex flex-col gap-2">
        <h1 className="text-3xl font-bold tracking-tight">API Key 连通性测试</h1>
        <p className="text-muted-foreground">
          直接调用网关接口测试 API Key 的有效性及模型响应效果。这是一个临时测试工具。
        </p>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
        {/* 配置面板 */}
        <Card className="md:col-span-1 h-fit">
          <CardHeader>
            <CardTitle className="text-lg flex items-center gap-2">
              <Key className="w-4 h-4" />
              测试配置
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="apiKey">API Key</Label>
              <Input
                id="apiKey"
                placeholder="sk-..."
                value={apiKey}
                onChange={e => setApiKey(e.target.value)}
                type="password"
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="modelName">模型名称</Label>
              <Input
                id="modelName"
                placeholder="gpt-4"
                value={modelName}
                onChange={e => setModelName(e.target.value)}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="userContent">测试内容</Label>
              <Textarea
                id="userContent"
                placeholder="输入你想测试的消息..."
                value={userContent}
                onChange={e => setUserContent(e.target.value)}
                rows={4}
              />
            </div>
            <Button className="w-full" onClick={handleTest} disabled={loading}>
              {loading ? (
                <>
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  测试中...
                </>
              ) : (
                <>
                  <Send className="mr-2 h-4 w-4" />
                  发送请求
                </>
              )}
            </Button>
          </CardContent>
        </Card>

        {/* 结果面板 */}
        <Card className="md:col-span-2 min-h-[400px]">
          <CardHeader>
            <CardTitle className="text-lg flex items-center gap-2">
              <Bot className="w-4 h-4" />
              响应内容
            </CardTitle>
            <CardDescription>
              {usage ? (
                <span className="text-xs">
                  Tokens: 输入 {usage.prompt_tokens} | 输出 {usage.completion_tokens} | 总计{' '}
                  {usage.total_tokens}
                </span>
              ) : (
                '点击发送请求后查看结果'
              )}
            </CardDescription>
          </CardHeader>
          <CardContent>
            {response ? (
              <div className="prose prose-sm dark:prose-invert max-w-none border rounded-lg p-4 bg-muted/30">
                <MarkdownViewer content={response} />
              </div>
            ) : (
              <div className="h-40 flex flex-col items-center justify-center text-muted-foreground border-2 border-dashed rounded-lg">
                <Bot className="w-8 h-8 opacity-20 mb-2" />
                <p className="text-sm">期待你的提问</p>
              </div>
            )}
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
