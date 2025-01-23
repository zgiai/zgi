import { createContext, useContext, useState, Dispatch, SetStateAction, useEffect } from "react"

// 定义更具体的类型
interface KnowledgeBase {
    id?: number;
    name?: string;
    description?: string;
    visibility?: "PUBLIC" | "PRIVATE";
    status?: number;
    collection_name?: string;
    model?: string;
    dimension?: number;
    document_count?: number;
    total_chunks?: number;
    total_tokens?: number;
    meta_info?: any;
    tags?: string[] | null;
    owner_id?: number;
    organization_id?: number | null;
    created_at?: string;
    updated_at?: string;
    owner_name?: string;
}

interface KnowledgeBaseContextProps {
    knowledgeBase: KnowledgeBase
    setKnowledgeBase: Dispatch<SetStateAction<KnowledgeBase>>
    update: boolean
    setUpdate: Dispatch<SetStateAction<boolean>>
}

export const KnowledgeBaseContext = createContext<KnowledgeBaseContextProps>({
    knowledgeBase: {},
    setKnowledgeBase: () => {},
    update: false,
    setUpdate: () => {}
})

export const KnowledgeBaseProvider = ({
    children
}: {
    children: React.ReactNode
}): JSX.Element => {
    const [knowledgeBase, setKnowledgeBase] = useState<KnowledgeBase>({})
    const [update, setUpdate] = useState(false)
    return <KnowledgeBaseContext.Provider value={{ knowledgeBase, setKnowledgeBase, update, setUpdate }}>
        {children}
    </KnowledgeBaseContext.Provider>
}

export const useKnowledgeBase = (): KnowledgeBaseContextProps => useContext(KnowledgeBaseContext)


