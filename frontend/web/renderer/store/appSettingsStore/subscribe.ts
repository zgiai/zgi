import { cloneDeep } from 'lodash'
import { useAppSettingsStore } from '.'
import type { ModelConfig } from './types'

const subscribeInit = () => {
  useAppSettingsStore.subscribe(
    (state) => state.isOpenModal,
    async () => {
      const { isOpenModal, saveSettings, loadSettings, generateModelsOptions } =
        useAppSettingsStore.getState()
      if (isOpenModal) {
        loadSettings()
        return
      }
      saveSettings()
      generateModelsOptions()
      useAppSettingsStore.setState({
        checkResults: {},
      })
    },
    { fireImmediately: false },
  )
}

export default subscribeInit
