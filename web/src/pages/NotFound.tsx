import { useNavigate } from 'react-router-dom'
import { PageState } from '@/components/ui/page-state'

export function NotFoundPage() {
  const navigate = useNavigate()

  return (
    <PageState
      title="Page not found"
      description="The route you requested does not exist in the tracing UI."
      actionLabel="Back to traces"
      onAction={() => navigate('/')}
      icon="404"
    />
  )
}
