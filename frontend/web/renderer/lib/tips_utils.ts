import { Bounce, type ToastOptions, toast } from 'react-toastify'

const baseToastOpts: ToastOptions = {
  position: 'top-center',
  autoClose: 1500,
  hideProgressBar: true,
  closeOnClick: false,
  pauseOnHover: true,
  draggable: true,
  progress: undefined,
  theme: 'light',
  transition: Bounce,
}

class Message {
  error(tips: string) {
    toast.error(tips, baseToastOpts)
  }
  warn(tips: string) {
    toast.warn(tips, baseToastOpts)
  }
  success(tips: string) {
    toast.success(tips, baseToastOpts)
  }
  info(tips: string) {
    toast.info(tips, baseToastOpts)
  }
}

export const message = new Message()
