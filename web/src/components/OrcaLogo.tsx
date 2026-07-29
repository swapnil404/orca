interface OrcaLogoProps {
  className?: string
  title?: string
}

export function OrcaLogo({ className = 'h-8 w-8', title }: OrcaLogoProps) {
  return (
    <svg
      viewBox="0 0 48 48"
      className={className}
      role={title ? 'img' : undefined}
      aria-hidden={title ? undefined : true}
      xmlns="http://www.w3.org/2000/svg"
    >
      {title && <title>{title}</title>}
      <path
        fill="var(--accent)"
        d="M23.7 27.2C17.2 18.4 10.4 15 3 16.8c3.1 8.9 10.2 14.5 20.7 16.5V42c2.2-2.9 3.5-5.9 3.8-9 9.3-2.5 15.2-8 17.5-16.5-7.1-1.1-13.2 2.3-17.9 10.3C26.4 17.4 23.5 9.8 18.4 4c-.9 9.3.9 17 5.3 23.2Z"
      />
      <path fill="var(--text)" d="M19.7 17.1c2.2 2.6 3.5 5.8 4 9.7-2.7-2.7-4.4-5.6-5-8.6-.3-1.3.2-1.7 1-1.1Z" />
    </svg>
  )
}
