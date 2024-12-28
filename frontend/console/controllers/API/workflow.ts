import { WorkFlow } from "@/types/flow";
import axios from "../request";

/**
 * 获取工作流节点模板s
 */
export const getWorkflowNodeTemplate = async (): Promise<any[]> => {
    return new Promise(res => setTimeout(() => {
        res(workflowTemplate)
    }, 100));
}

/**
 * 获取某工作流报告模板信息
 */
export const getWorkflowReportTemplate = async (key: string): Promise<any> => {
    return await axios.get(`/api/v1/workflow/report/file?version_key=${key}`);
}

/**
 * 创建工作流
 */
export const createWorkflowApi = async (name, desc, url, flow): Promise<any> => {
    if (url) {
        // logo保存相对路径
        url = url.match(/(icon.*)\?/)?.[1]
    }
    const data = flow || {}
    return await axios.post("/api/v1/workflow/create", {
        ...data,
        name,
        description: desc,
        logo: url
    });
}

/**
 * 保存工作流
 */
export const saveWorkflow = async (versionId: number, data: WorkFlow): Promise<any> => {
    if (data.logo) {
        // logo保存相对路径
        data.logo = data.logo.match(/(icon.*)\?/)?.[1]
    }
    return await axios.put(`/api/v1/workflow/versions/${versionId}`, data);
}

/** 上线工作流 & 修改信息 
 * status: 2 上线 1 下线
*/
export const onlineWorkflow = async (flow, status = ''): Promise<any> => {
    const { name, description, logo } = flow
    const data = { name, description, logo: logo && logo.match(/(icon.*)\?/)?.[1] }
    if (status) {
        data['status'] = status
    }
    return await axios.patch(`/api/v1/workflow/update/${flow.id}`, data);
}


/**
 * 工作流节点模板
 */
const workflowTemplate = [
    {
        "id": "start_xxx",
        "name": "开始",
        "description": "工作流运行的起始节点。",
        "type": "start",
        "group_params": [
            {
                "name": "开场引导",
                "params": [
                    {
                        "key": "guide_word",
                        "label": "开场白",
                        "value": "",
                        "type": "textarea",
                        "placeholder": "每次工作流开始执行时向用户发送此消息，支持 Markdown 格式，为空时不发送。"
                    },
                    {
                        "key": "guide_question",
                        "label": "引导问题",
                        "value": [],
                        "type": "input_list",
                        "placeholder": "请输入引导问题",
                        "help": "为用户提供推荐问题，引导用户输入，超过3个时将随机选取3个。"
                    }
                ]
            },
            {
                "name": "全局变量",
                "params": [
                    {
                        "key": "current_time",
                        "global": "key",
                        "label": "当前时间",
                        "type": "var",
                        "value": ""
                    },
                    {
                        "key": "chat_history",
                        "global": "key",
                        "type": "chat_history_num",
                        "value": 10
                    },
                    {
                        "key": "preset_question",
                        "label": "预置问题列表",
                        "global": "index",
                        "type": "input_list",
                        "value": [],
                        "placeholder": "输入批量预置问题",
                        "help": "适合文档审核、报告生成等场景，利用提前预置的问题批量进行 RAG 问答。"
                    }
                ]
            }
        ]
    },
    {
        "id": "input_xxx",
        "name": "输入",
        "description": "接收用户在会话页面的输入，支持 2 种形式：对话框输入，表单输入。",
        "type": "input",
        "tab": {
            "value": "input",
            "options": [
                {
                    "label": "对话框输入",
                    "key": "input",
                    "help": "接收用户从对话框输入的内容。!(input)"
                },
                {
                    "label": "表单输入",
                    "key": "form",
                    "help": "将会在用户会话界面弹出一个表单，接收用户从表单提交的内容。!(form)"
                }
            ]
        },
        "group_params": [
            {
                "name": "",
                "params": [
                    {
                        "key": "user_input",
                        "global": "key",
                        "label": "用户输入内容",
                        "type": "var",
                        "tab": "input"
                    },
                    {
                        "global": "code:value.map(el => ({ label: el.key, value: el.key }))",
                        "key": "form_input",
                        "label": "+ 添加表单项",
                        "type": "form",
                        "value": [],
                        "tab": "form"
                    }
                ]
            }
        ]
    },
    {
        "id": "output_xxx",
        "name": "输出",
        "description": "可向用户发送消息，并且支持进行更丰富的交互，例如请求用户批准进行某项敏感操作、允许用户在模型输出内容的基础上直接修改并提交。",
        "type": "output",
        "group_params": [
            {
                "params": [
                    {
                        "key": "output_msg",
                        "label": "消息内容",
                        "type": "var_textarea_file",
                        "required": true,
                        "placeholder": "输入需要发送给用户的消息，例如“接下来我将执行 XX 操作，请您确认”，“以下是我的初版草稿，您可以在其基础上进行修改”",
                        "value": {
                            "msg": "",
                            "files": []
                        }
                    },
                    {
                        "key": "output_result",
                        "label": "交互类型",
                        "global": "value.type=input",
                        "type": "output_form",
                        "required": true,
                        "value": {
                            "type": "",
                            "value": ""
                        },
                        "options": []
                    }
                ]
            }
        ]
    },
    {
        "id": "llm_xxx",
        "name": "大模型",
        "description": "调用大模型回答用户问题或者处理任务。",
        "type": "llm",
        "tab": {
            "value": "single",
            "options": [
                {
                    "label": "单次运行",
                    "key": "single"
                },
                {
                    "label": "批量运行",
                    "key": "batch"
                }
            ]
        },
        "group_params": [
            {
                "params": [
                    {
                        "key": "batch_variable",
                        "label": "批处理变量",
                        "global": "self",
                        "type": "user_question",
                        "value": [],
                        "required": true,
                        "linkage": "output",
                        "placeholder": "请选择批处理变量",
                        "help": "选择需要批处理的变量，将会多次运行本节点，每次运行时从选择的变量中取一项赋值给batch_variable进行处理。",
                        "tab": "batch"
                    }
                ]
            },
            {
                "name": "模型设置",
                "params": [
                    {
                        "key": "model_id",
                        "label": "模型",
                        "type": "bisheng_model",
                        "value": "",
                        "required": true,
                        "placeholder": "请选择模型"
                    },
                    {
                        "key": "temperature",
                        "label": "温度",
                        "type": "slide",
                        "scope": [
                            0,
                            2
                        ],
                        "step": 0.1,
                        "value": 0.7
                    }
                ]
            },
            {
                "name": "提示词",
                "params": [
                    {
                        "key": "system_prompt",
                        "label": "系统提示词",
                        "type": "var_textarea",
                        "test": "input",
                        "value": ""
                    },
                    {
                        "key": "user_prompt",
                        "label": "用户提示词",
                        "type": "var_textarea",
                        "test": "input",
                        "value": "",
                        "required": true
                    }
                ]
            },
            {
                "name": "输出",
                "params": [
                    {
                        "key": "output_user",
                        "label": "将输出结果展示在会话中",
                        "type": "switch",
                        "help": "一般在问答等场景可开启，文档审核、报告生成等场景可关闭。",
                        "value": true
                    },
                    {
                        "key": "output",
                        "global": "code:value.map(el => ({ label: el.label, value: el.key }))",
                        "label": "输出变量",
                        "type": "var",
                        "value": []
                    }
                ]
            }
        ]
    },
    {
        "id": "agent_xxx",
        "name": "助手",
        "description": "AI 自主进行任务规划，选择合适的知识库或工具进行调用。",
        "type": "agent",
        "tab": {
            "value": "single",
            "options": [
                {
                    "label": "单次运行",
                    "key": "single"
                },
                {
                    "label": "批量运行",
                    "key": "batch"
                }
            ]
        },
        "group_params": [
            {
                "params": [
                    {
                        "key": "batch_variable",
                        "label": "批处理变量",
                        "required": true,
                        "global": "self",
                        "type": "user_question",
                        "value": [],
                        "linkage": "output",
                        "placeholder": "请选择批处理变量",
                        "tab": "batch",
                        "help": "选择需要批处理的变量，将会多次运行本节点，每次运行时从选择的变量中取一项赋值给batch_variable进行处理。"
                    }
                ]
            },
            {
                "name": "模型设置",
                "params": [
                    {
                        "key": "model_id",
                        "label": "模型",
                        "type": "agent_model",
                        "required": true,
                        "value": "",
                        "placeholder": "请选择模型"
                    },
                    {
                        "key": "temperature",
                        "label": "温度",
                        "type": "slide",
                        "scope": [
                            0,
                            2
                        ],
                        "step": 0.1,
                        "value": 0.7
                    }
                ]
            },
            {
                "name": "提示词",
                "params": [
                    {
                        "key": "system_prompt",
                        "label": "系统提示词",
                        "type": "var_textarea",
                        "test": "input",
                        "value": "",
                        "placeholder": "助手画像",
                        "required": true
                    },
                    {
                        "key": "user_prompt",
                        "label": "用户提示词",
                        "type": "var_textarea",
                        "test": "input",
                        "value": "",
                        "placeholder": "用户消息内容",
                        "required": true
                    },
                    {
                        "key": "chat_history_flag",
                        "label": "历史聊天记录",
                        "type": "slide_switch",
                        "scope": [
                            0,
                            100
                        ],
                        "step": 1,
                        "value": {
                            "flag": false,
                            "value": 50
                        },
                        "help": "是否携带历史对话记录。"
                    }
                ]
            },
            {
                "name": "知识库",
                "params": [
                    {
                        "key": "knowledge_id",
                        "label": "检索知识库范围",
                        "type": "knowledge_select_multi",
                        "placeholder": "请选择知识库",
                        "value": {
                            "type": "knowledge",
                            "value": []
                        }
                    }
                ]
            },
            {
                "name": "工具",
                "params": [
                    {
                        "key": "tool_list",
                        "label": "+ 添加工具",
                        "type": "add_tool",
                        "value": []
                    }
                ]
            },
            {
                "name": "输出",
                "params": [
                    {
                        "key": "output_user",
                        "label": "将输出结果展示在会话中",
                        "type": "switch",
                        "help": "一般在问答等场景开启，文档审核、报告生成等场景可关闭。",
                        "value": false
                    },
                    {
                        "key": "output",
                        "global": "code:value.map(el => ({ label: el.label, value: el.key }))",
                        "label": "输出变量",
                        "type": "var",
                        "value": []
                    }
                ]
            }
        ]
    },
    {
        "id": "qa_retriever_xxx",
        "name": "QA知识库检索",
        "description": "从 QA 知识库中检索问题以及对应的答案。",
        "type": "qa_retriever",
        "group_params": [
            {
                "name": "检索设置",
                "params": [
                    {
                        "key": "user_question",
                        "label": "输入变量",
                        "type": "var_select",
                        "test": "input",
                        "value": "",
                        "required": true,
                        "placeholder": "请选择检索问题"
                    },
                    {
                        "key": "qa_knowledge_id",
                        "label": "QA知识库",
                        "type": "qa_select_multi",
                        "value": [],
                        "required": true,
                        "placeholder": "请选择QA知识库"
                    },
                    {
                        "key": "score",
                        "label": "相似度阈值",
                        "type": "slide",
                        "value": 0.6,
                        "scope": [
                            0.01,
                            0.99
                        ],
                        "step": 0.01,
                        "help": "低于阈值的结果将会被过滤。"
                    }
                ]
            },
            {
                "name": "输出",
                "params": [
                    {
                        "key": "retrieval_result",
                        "label": "检索结果",
                        "type": "var",
                        "global": "key",
                        "value": ""
                    }
                ]
            }
        ]
    },
    {
        "id": "rag_xxx",
        "name": "文档知识库问答",
        "description": "根据用户问题从知识库中检索相关内容，结合检索结果调用大模型生成最终结果，支持多个问题并行执行。",
        "type": "rag",
        "group_params": [
            {
                "name": "知识库检索设置",
                "params": [
                    {
                        "key": "user_question",
                        "label": "用户问题",
                        "global": "self=system_prompt,user_prompt",
                        "type": "user_question",
                        "test": "input",
                        "help": "当选择多个问题时，将会多次运行本节点，每次运行时从批量问题中取一项进行处理。",
                        "linkage": "output_user_input",
                        "value": [],
                        "placeholder": "请选择用户问题",
                        "required": true
                    },
                    {
                        "key": "retrieved_result",
                        "label": "检索范围",
                        "global": "self=system_prompt,user_prompt",
                        "type": "knowledge_select_multi",
                        "value": {
                            "type": "knowledge",
                            "value": []
                        },
                        "required": true
                    },
                    {
                        "key": "user_auth",
                        "label": "用户知识库权限校验",
                        "type": "switch",
                        "value": false,
                        "help": "开启后，只会对用户有使用权限的知识库进行检索。"
                    },
                    {
                        "key": "max_chunk_size",
                        "label": "检索结果长度",
                        "type": "number",
                        "value": 15000,
                        "help": "通过此参数控制最终传给模型的知识库检索结果文本长度，超过模型支持的最大上下文长度可能会导致报错。"
                    }
                ]
            },
            {
                "name": "AI回复生成设置",
                "params": [
                    {
                        "key": "system_prompt",
                        "label": "系统提示词",
                        "type": "var_textarea",
                        "value": "你是一个知识库问答助手： \n1.用中文回答用户问题，并且答案要严谨专业。\n2.你需要依据以上【参考文本】中的内容来回答，当【参考文本】中有明确与用户问题相关的内容时才进行回答，不可根据自己的知识来回答。\n3.由于【参考文本】可能包含多个来自不同信息源的信息，所以根据这些不同的信息源可能得出有差异甚至冲突的答案，当发现这种情况时，这些答案都列举出来；如果没有冲突或差异，则只需要给出一个最终结果。\n4.若【参考文本】中内容与用户问题不相关则回复“没有找到相关内容”。",
                        "required": true
                    },
                    {
                        "key": "user_prompt",
                        "label": "用户提示词",
                        "type": "var_textarea",
                        "value": "用户问题：{{#user_question#}}\n参考文本：{{#retrieved_result#}}\n你的回答：",
                        "required": true
                    },
                    {
                        "key": "model_id",
                        "label": "模型",
                        "type": "bisheng_model",
                        "value": "",
                        "required": true,
                        "placeholder": "请选择模型"
                    },
                    {
                        "key": "temperature",
                        "label": "温度",
                        "type": "slide",
                        "scope": [
                            0,
                            2
                        ],
                        "step": 0.1,
                        "value": 0.7
                    }
                ]
            },
            {
                "name": "输出",
                "params": [
                    {
                        "key": "output_user",
                        "label": "将输出结果展示在会话中",
                        "type": "switch",
                        "value": true,
                        "help": "一般在问答等场景开启，文档审核、报告生成等场景可关闭。"
                    },
                    {
                        "key": "output_user_input",
                        "label": "输出变量",
                        "type": "var",
                        "global": "code:value.map(el => ({ label: el.label, value: el.key }))",
                        "value": []
                    }
                ]
            }
        ]
    },
    {
        "id": "report_xxx",
        "name": "报告",
        "description": "按照预设的word模板生成报告。",
        "type": "report",
        "group_params": [
            {
                "params": [
                    {
                        "key": "report_info",
                        "label": "报告名称",
                        "placeholder": "请输入生成报告的名称",
                        "required": true,
                        "type": "report",
                        "value": {}
                    }
                ]
            }
        ]
    },
    {
        "id": "code_xxx",
        "name": "代码",
        "description": "自定义需要执行的代码。",
        "type": "code",
        "group_params": [
            {
                "name": "入参",
                "params": [
                    {
                        "key": "code_input",
                        "type": "code_input",
                        "test": "input",
                        "required": true,
                        "value": [
                            {
                                "key": "arg1",
                                "type": "input",
                                "label": "",
                                "value": ""
                            },
                            {
                                "key": "arg2",
                                "type": "input",
                                "label": "",
                                "value": ""
                            }
                        ]
                    }
                ]
            },
            {
                "name": "执行代码",
                "params": [
                    {
                        "key": "code",
                        "type": "code",
                        "required": true,
                        "value": "def main(arg1: str, arg2: str) -> dict: \n    return {'result1': arg1, 'result2': arg2}"
                    }
                ]
            },
            {
                "name": "出参",
                "params": [
                    {
                        "key": "code_output",
                        "type": "code_output",
                        "global": "code:value.map(el => ({ label: el.key, value: el.key }))",
                        "required": true,
                        "value": [
                            {
                                "key": "result1",
                                "type": "str"
                            },
                            {
                                "key": "result2",
                                "type": "str"
                            }
                        ]
                    }
                ]
            }
        ]
    },
    {
        "id": "condition_xxx",
        "name": "条件分支",
        "description": "根据条件表达式执行不同的分支。",
        "type": "condition",
        "group_params": [
            {
                "params": [
                    {
                        "key": "condition",
                        "label": "",
                        "type": "condition",
                        "value": []
                    }
                ]
            }
        ]
    },
    {
        "id": "end_xxx",
        "name": "结束",
        "description": "工作流运行到此结束。",
        "type": "end",
        "group_params": []
    },
]