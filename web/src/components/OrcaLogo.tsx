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
        d="M5 24c0-7.5 7-12.8 17.2-12.8 4.7 0 9.5 1.5 13.4 4.5 3-2.6 6.1-3.8 9.4-3.7-.8 4-2.8 7.2-5.9 9.4 3 2.2 5 5.4 5.9 9.4-3.5.1-6.8-1.3-9.8-4.2-4.1 4.5-9.1 7-15.2 7C11.3 33.6 5 29.7 5 24Zm14.2-11.6C20.4 7.2 23.5 3.7 28.5 1c1.4 5 1.3 9.4-.4 13.2-2.9-1.2-5.8-1.8-8.9-1.8Z"
      />
      <path fill="var(--text)" d="M10.7 20.3c3-3.1 7.2-4.5 11.8-3.8-1.8.9-3.4 2.5-4.6 4.6-2.9 1.2-5.3.9-7.2-.8Zm2 6.7c3.9 2.1 8 2.4 12.2.9-2.1 2.1-4.8 3.1-8 2.8-2.3-.2-3.7-1.5-4.2-3.7Z" />
      <circle cx="10.6" cy="22.1" r="1.25" fill="var(--bg)" />
    </svg>
  )
}
