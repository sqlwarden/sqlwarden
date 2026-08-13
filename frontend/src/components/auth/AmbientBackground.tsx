export function AmbientBackground() {
  return (
    <div
      aria-hidden="true"
      className="pointer-events-none fixed inset-0 -z-10 overflow-hidden bg-background"
    >
      <svg
        className="absolute inset-0 size-full text-muted-foreground"
        fill="none"
        preserveAspectRatio="xMidYMid slice"
        viewBox="0 0 1600 1000"
      >
        <defs>
          <path
            d="M-440-38C-417-195-286-310-126-304 19-299 99-231 205-157c105 74 243 143 266 267 24 128-69 262-196 302-116 37-208-30-317-49-132-23-294 16-377-89-81-102-43-237-21-312Z"
            id="auth-contour-a"
          />
          <path
            d="M-426-79c54-148 203-226 353-199 126 22 176 120 271 189 108 78 267 119 283 251 15 123-91 232-211 262-113 28-202-39-310-58-129-22-279 15-364-83-83-96-68-242-22-362Z"
            id="auth-contour-b"
          />
          <path
            d="M-453-22c22-142 145-266 290-280 139-14 236 77 340 162 102 83 254 161 258 292 4 120-101 218-216 254-121 38-211-17-326-43-132-30-295-15-360-132-43-78-1-170 14-253Z"
            id="auth-contour-c"
          />
          <radialGradient id="auth-contour-clear" cx="50%" cy="48%" r="62%">
            <stop offset="0%" stopColor="black" />
            <stop offset="34%" stopColor="black" />
            <stop offset="72%" stopColor="white" />
            <stop offset="100%" stopColor="white" />
          </radialGradient>
          <mask id="auth-contour-mask">
            <rect width="1600" height="1000" fill="url(#auth-contour-clear)" />
          </mask>
        </defs>

        <g
          mask="url(#auth-contour-mask)"
          stroke="currentColor"
          strokeLinecap="round"
          strokeLinejoin="round"
          strokeWidth="1.1"
          vectorEffect="non-scaling-stroke"
        >
          <g opacity="0.16" transform="translate(105 205)">
            <use href="#auth-contour-a" transform="scale(.34)" />
            <use href="#auth-contour-b" transform="scale(.47)" />
            <use href="#auth-contour-c" transform="scale(.61)" />
            <use href="#auth-contour-a" transform="scale(.76)" />
            <use href="#auth-contour-b" transform="scale(.92)" />
            <use href="#auth-contour-c" transform="scale(1.09)" />
            <use href="#auth-contour-a" transform="scale(1.27)" />
            <use href="#auth-contour-b" transform="scale(1.46)" />
          </g>

          <g opacity="0.13" transform="translate(1515 805) scale(-1 1)">
            <use href="#auth-contour-c" transform="scale(.3)" />
            <use href="#auth-contour-a" transform="scale(.43)" />
            <use href="#auth-contour-b" transform="scale(.57)" />
            <use href="#auth-contour-c" transform="scale(.72)" />
            <use href="#auth-contour-a" transform="scale(.89)" />
            <use href="#auth-contour-b" transform="scale(1.07)" />
            <use href="#auth-contour-c" transform="scale(1.26)" />
            <use href="#auth-contour-a" transform="scale(1.47)" />
          </g>

          <g opacity="0.1" transform="translate(1235 30) rotate(19)">
            <use href="#auth-contour-b" transform="scale(.2)" />
            <use href="#auth-contour-c" transform="scale(.31)" />
            <use href="#auth-contour-a" transform="scale(.43)" />
            <use href="#auth-contour-b" transform="scale(.56)" />
          </g>

          <g stroke="var(--primary)" strokeWidth="1.35">
            <use href="#auth-contour-a" opacity="0.18" transform="translate(105 205) scale(.76)" />
            <use
              href="#auth-contour-c"
              opacity="0.14"
              transform="translate(1515 805) scale(-.72 .72)"
            />
          </g>

          <g stroke="var(--chart-3)" strokeWidth="1.2">
            <use href="#auth-contour-b" opacity="0.12" transform="translate(105 205) scale(1.09)" />
            <use
              href="#auth-contour-a"
              opacity="0.1"
              transform="translate(1235 30) rotate(19) scale(.43)"
            />
          </g>
        </g>
      </svg>

      <div className="absolute inset-x-0 bottom-0 h-1/3 bg-linear-to-b from-transparent to-background/60" />
    </div>
  )
}
