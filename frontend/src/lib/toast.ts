import { toast as sonnerToast } from 'sonner'

export const toast = {
  success: (message: string) => sonnerToast.success(message),
  error: (message: string) => sonnerToast.error(message),
  info: (message: string) => sonnerToast(message),
  loading: (message: string) => sonnerToast.loading(message),
  dismiss: (id?: string | number) => sonnerToast.dismiss(id),
}
