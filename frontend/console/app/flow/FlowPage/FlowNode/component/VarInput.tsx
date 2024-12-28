import { Button } from "@/components/bs-ui/button";
import { Label } from "@/components/bs-ui/label";
import { isVarInFlow } from "@/utils/flowUtils";
import { UploadCloud, Variable } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import useFlowStore from "../../flowStore";
import SelectVar from "./SelectVar";
import { RbDragIcon } from "@/components/bs-icons/rbDrag";

// 解析富文本内容为保存格式
function parseToValue(input, flowNode) {
    const tempDiv = document.createElement('div');
    tempDiv.innerHTML = input.replace(
        /<span[^>]*?>(.*?)<\/span>/g, // 匹配所有 <span> 标签
        (match, content) => {
            // 在对象中查找匹配的 key
            const key = Object.keys(flowNode.varZh).find((k) => flowNode.varZh[k] === content);
            return key ? `{{#${key}#}}` : content; // 如果找到 key，返回 key，否则保持原内容
        }
    );

    // 遍历子节点，将 <br> 转换为 \n，同时处理文本内容
    const traverseNodes = (node) => {
        let result = '';
        node.childNodes.forEach((child) => {
            if (child.nodeName === 'BR') {
                result += '\n'; // 换行符
            } else if (child.nodeType === Node.TEXT_NODE) {
                result += child.textContent; // 文本内容
            } else if (child.nodeType === Node.ELEMENT_NODE) {
                result += traverseNodes(child); // 递归解析子元素
            }
        });
        return result;
    };
    return traverseNodes(tempDiv);
}


export default function VarInput({
    nodeId,
    itemKey,
    placeholder = '',
    flowNode,
    value,
    error = false,
    children = null,
    onUpload = undefined,
    onChange,
    onVarEvent = undefined,
}) {
    const { textareaRef, handleFocus, handleBlur } = usePlaceholder(placeholder);
    const valueRef = useRef(value || '');
    const selectVarRef = useRef(null);

    const { flow } = useFlowStore();

    // 校验变量是否可用
    const validateVarAvailble = () => {
        const value = valueRef.current;
        const [html, error] = parseToHTML(value || '', true);
        textareaRef.current.innerHTML = html;
        return error;
    };

    useEffect(() => {
        onVarEvent && onVarEvent(validateVarAvailble);
        return () => onVarEvent && onVarEvent(() => { });
    }, [flowNode]);

    const handleInput = () => {
        const value = parseToValue(textareaRef.current.innerHTML, flowNode);
        // console.log('textarea value :>> ', value);
        valueRef.current = value;
        onChange(value);
    };

    function parseToHTML(input, validate = false) {
        let error = '';
        const html = input
            .replace(/{{#(.*?)#}}/g, (a, part) => {
                if (validate) {
                    error = isVarInFlow(nodeId, flow.nodes, part, flowNode.varZh?.[part]);
                }
                const msgZh = flowNode.varZh?.[part] || part;
                return `<span class=${error ? 'textarea-error' : 'textarea-badge'} contentEditable="false">${msgZh}</span>`;
            })
            .replace(/\n/g, '<br>');
        return [html, error];
    }

    useEffect(() => {
        // console.log('value :>> ', value);
        textareaRef.current.innerHTML = parseToHTML(value || '')[0];
        handleBlur();
    }, []);

    // 在光标位置插入内容
    function handleInsertVariable(item, _var) {
        handleFocus();

        const selection = window.getSelection();
        let range = selection.getRangeAt(0);
        if (!selection.rangeCount) return;

        if (!textareaRef.current.contains(range.commonAncestorContainer)) {
            range = document.createRange();
            range.selectNodeContents(textareaRef.current); // 设置范围为富文本框内容
            range.collapse(false); // 将光标定位到内容末尾
            selection.removeAllRanges();
            selection.addRange(range);
        }

        // 文本框内容
        const key = `${item.id}.${_var.value}`;
        const label = `${item.name}/${_var.label}`;
        if (flowNode.varZh) {
            flowNode.varZh[key] = label;
        } else {
            flowNode.varZh = {
                [key]: label,
            };
        }

        const html = `<span class="textarea-badge" contentEditable="false">${label}</span>`;
        const fragment = range.createContextualFragment(html);

        const lastChild = fragment.lastChild; // 提前保存引用
        if (lastChild) {
            range.deleteContents();
            range.insertNode(fragment);

            // 现在用保存的引用而不是 fragment.lastChild
            range.setStartAfter(lastChild);
            range.setEndAfter(lastChild);
            selection.removeAllRanges();
            selection.addRange(range);
        } else {
            console.warn('No valid child nodes to insert.');
        }

        handleInput();
    }

    const handlePaste = (e) => {
        // fomat text
        e.preventDefault(); // 阻止默认粘贴行为
        const text = e.clipboardData.getData('text'); // 从剪贴板中获取纯文本内容
        document.execCommand('insertText', false, text);
    };
    // resize
    const { height, handleMouseDown } = useResize(textareaRef, 80, 40);

    return (
        <div
            className={`nodrag mt-2 flex flex-col w-full relative rounded-md border bg-search-input text-sm shadow-sm ${error ? 'border-red-500' : 'border-input'
                }`}
        >
            <div className="flex justify-between gap-1 border-b px-2 py-1" onClick={() => textareaRef.current.focus()}>
                <Label className="bisheng-label text-xs" onClick={validateVarAvailble}>
                    变量输入
                </Label>
                <div className="flex gap-2">
                    <SelectVar ref={selectVarRef} nodeId={nodeId} itemKey={itemKey} onSelect={handleInsertVariable}>
                        <Variable size={16} className="text-muted-foreground hover:text-gray-800" />
                    </SelectVar>
                    {onUpload && (
                        <Button variant="ghost" className="p-0 h-4 text-muted-foreground" onClick={onUpload}>
                            <UploadCloud size={16} />
                        </Button>
                    )}
                </div>
            </div>
            <div
                ref={textareaRef}
                contentEditable
                style={{ height }}
                onInput={handleInput}
                onPaste={handlePaste}
                onFocus={handleFocus}
                onBlur={handleBlur}
                onKeyDown={(e) => {
                    // 唤起插入变量
                    if (e.key === '{') {
                        selectVarRef.current.open();
                        e.preventDefault();
                    }
                }}
                className="nowheel bisheng-richtext px-3 py-2 whitespace-pre-line min-h-[80px] max-h-64 overflow-y-auto overflow-x-hidden border-none outline-none bg-search-input rounded-md dark:text-gray-50 placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50"
            ></div>
            {children}
            <div
                className="resize-handle w-4 h-4 bg-transparent absolute cursor-ns-resize right-0 bottom-0 rounded-ee-md overflow-hidden"
                onMouseDown={handleMouseDown}
            ><RbDragIcon /></div>
        </div>
    );
}

const useResize = (textareaRef, initialHeight = 80, minHeight = 40) => {
    const [height, setHeight] = useState(initialHeight); // 初始高度

    // 处理拖拽调整高度
    const handleDrag = (e) => {
        e.preventDefault();
        const newHeight = e.clientY - textareaRef.current.getBoundingClientRect().top;
        if (newHeight > minHeight) {
            setHeight(newHeight); // 更新高度
        }
    };

    const handleMouseUp = () => {
        document.removeEventListener('mousemove', handleDrag); // 停止拖拽
        document.removeEventListener('mouseup', handleMouseUp);
    };

    const handleMouseDown = (e) => {
        document.addEventListener('mousemove', handleDrag); // 开始拖拽
        document.addEventListener('mouseup', handleMouseUp);
    };

    useEffect(() => {
        // 在组件卸载时清理事件监听器
        return () => {
            document.removeEventListener('mousemove', handleDrag);
            document.removeEventListener('mouseup', handleMouseUp);
        };
    }, []);

    return {
        height,
        textareaRef,
        handleMouseDown
    };
};


function usePlaceholder(placeholder) {
    const divRef = useRef(null);

    const handleFocus = () => {
        if (divRef.current && divRef.current.innerHTML.trim() === placeholder) {
            divRef.current.innerHTML = "";
            divRef.current.classList.remove("placeholder");
        }
    };

    const handleBlur = () => {
        if (divRef.current && ['<br>', ''].includes(divRef.current.innerHTML.trim())) {
            divRef.current.innerHTML = placeholder;
            divRef.current.classList.add("placeholder");
        }
    };

    useEffect(() => {
        if (!placeholder) return
        if (divRef.current) {
            // 添加事件监听
            divRef.current.addEventListener("focus", handleFocus);
            divRef.current.addEventListener("blur", handleBlur);

            // 清理事件监听
            return () => {
                if (divRef.current) {
                    divRef.current.removeEventListener("focus", handleFocus);
                    divRef.current.removeEventListener("blur", handleBlur);
                }
            };
        }
    }, [placeholder]);

    return { textareaRef: divRef, handleFocus, handleBlur };
}


