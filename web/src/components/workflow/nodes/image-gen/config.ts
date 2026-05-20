import type { CommonNodeType, PromptVariableSelector } from '../../store/type';

export interface ImageGenNodeData {
  type: 'image-gen';
  title: string;
  desc?: string;
  model: {
    provider: string;
    name: string;
  };
  prompt: string;
  prompt_config: {
    template_variables: PromptVariableSelector[];
  };
  generation: {
    n: number;
    size: string;
    quality?: string;
  };
  output: {
    lifecycle: 'persistent' | 'temporary';
  };
  retry_config?: {
    enable: boolean;
    max_times: number;
    interval: number;
  };
  error_strategy?: 'fail-branch';
  default_value?: unknown[];
  isInLoop: boolean;
  isInIteration: boolean;
}

export type ImageGenNodeType = CommonNodeType & {
  data: ImageGenNodeData;
};

export const DEFAULT_IMAGE_GEN_NODE_DATA: ImageGenNodeData = {
  type: 'image-gen',
  title: 'Image Generation',
  desc: '',
  model: {
    provider: 'qwen',
    name: 'qwen-image-2.0',
  },
  prompt: '',
  prompt_config: {
    template_variables: [],
  },
  generation: {
    n: 1,
    size: '1024x1024',
    quality: '',
  },
  output: {
    lifecycle: 'persistent',
  },
  retry_config: {
    enable: false,
    max_times: 0,
    interval: 0,
  },
  error_strategy: 'fail-branch',
  default_value: [],
  isInLoop: false,
  isInIteration: false,
};
