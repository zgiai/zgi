import { useEffect } from "react";
import Chat from "./Chat";
import { useMessageStore } from "./messageStore";

export default function ChatPane({ autoRun = false, chatId, flow, wsUrl = '' }: { autoRun?: boolean, chatId: string, flow: any, wsUrl?: string }) {
    const { changeChatId } = useMessageStore()

    useEffect(() => {
        changeChatId(chatId)
    }, [chatId])

    const getMessage = (action, { nodeId, msg, category, extra, source, message_id }) => {
        if (action === 'getInputForm') {
            const node = flow.nodes.find(node => node.id === nodeId)
            if (node.data.tab.value === 'input') return null
            let form = null
            node.data.group_params.some(group => group.params.some(param => {
                if (param.tab === 'form') {
                    form = param
                    return true
                }
                return false
            }))
            return form
        }
        if (action === 'input') {
            const node = flow.nodes.find(node => node.id === nodeId)
            const tab = node.data.tab.value
            let variable = ''
            node.data.group_params.some(group =>
                group.params.some(param => {
                    if (param.tab === tab) {
                        variable = param.key
                        return true
                    }
                    return false
                })
            )
            return {
                action,
                data: {
                    [nodeId]: {
                        data: {
                            [variable]: msg
                        },
                        message: msg,
                        message_id,
                        category,
                        extra,
                        source
                    }
                }
            }
        }

        return {
            action,
            data: flow
        }
    }

    return <Chat
        autoRun={autoRun}
        useName=''
        guideWord=''
        clear
        wsUrl={wsUrl}
        onBeforSend={getMessage}
    ></Chat>

};
