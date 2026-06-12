import { computed } from 'vue'

export type GlassLevel = 'none' | 'subtle' | 'standard' | 'heavy' | 'extreme'

const glassMap: Record<GlassLevel, { bg: string; blur: string }> = {
  none: { bg: 'rgba(255,255,255,1)', blur: '0px' },
  subtle: { bg: 'rgba(255,255,255,0.85)', blur: '16px' },
  standard: { bg: 'rgba(255,255,255,0.65)', blur: '32px' },
  heavy: { bg: 'rgba(255,255,255,0.45)', blur: '48px' },
  extreme: { bg: 'rgba(255,255,255,0.25)', blur: '64px' },
}

export function useGlass(level: GlassLevel = 'standard') {
  const style = computed(() => {
    const g = glassMap[level]
    return {
      background: g.bg,
      backdropFilter: `blur(${g.blur}) saturate(1.2)`,
      WebkitBackdropFilter: `blur(${g.blur}) saturate(1.2)`,
    }
  })

  const className = computed(() => `glass-${level}`)

  return { style, className }
}
