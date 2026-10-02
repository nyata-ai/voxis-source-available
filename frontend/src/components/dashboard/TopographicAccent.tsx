import { cn } from '@/lib/utils'

interface TopographicAccentProps {
  className?: string
}

export function TopographicAccent({ className }: TopographicAccentProps) {
  return (
    <svg
      viewBox="0 0 800 28"
      preserveAspectRatio="none"
      className={cn('h-7 w-full text-border', className)}
      aria-hidden="true"
    >
      <path
        d="M0 6 Q 100 2 200 6 T 400 6 T 600 7 T 800 5"
        stroke="currentColor"
        strokeWidth={0.75}
        fill="none"
        opacity={0.7}
      />
      <path
        d="M0 14 Q 120 11 240 14 T 480 15 T 720 13 T 800 14"
        stroke="currentColor"
        strokeWidth={0.75}
        fill="none"
        strokeDasharray="2 4"
        opacity={0.5}
      />
      <path
        d="M0 22 Q 160 19 320 22 T 640 22 T 800 21"
        stroke="currentColor"
        strokeWidth={0.5}
        fill="none"
        opacity={0.35}
      />
    </svg>
  )
}
