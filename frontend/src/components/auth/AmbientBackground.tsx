import './ambient-background.css'

export function AmbientBackground() {
  return (
    <div
      aria-hidden="true"
      className="pointer-events-none fixed inset-0 -z-10 overflow-hidden bg-background"
    >
      <div className="auth-mesh-field-a absolute top-[0%] left-1/2 h-[52rem] w-[52rem] -ml-[26rem] blur-[120px]">
        <div
          className="size-full"
          style={{
            background: 'radial-gradient(55% 50% at 50% 45%, var(--primary), transparent 72%)',
          }}
        />
      </div>

      <div className="auth-mesh-field-b absolute top-[22%] left-[16%] h-[38rem] w-[38rem] blur-[110px]">
        <div
          className="size-full"
          style={{
            background: 'radial-gradient(50% 50% at 50% 50%, var(--chart-3), transparent 70%)',
          }}
        />
      </div>

      <div className="auth-mesh-field-c absolute top-[38%] right-[10%] h-[34rem] w-[34rem] blur-[100px]">
        <div
          className="size-full"
          style={{
            background: 'radial-gradient(50% 50% at 50% 50%, var(--primary), transparent 70%)',
          }}
        />
      </div>

      <div className="auth-ambient-bottom-fade absolute inset-x-0 bottom-0 h-[45%]" />
    </div>
  )
}
