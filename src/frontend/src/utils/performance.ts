// 性能监控工具：收集和分析应用性能指标
// 支持性能计时、指标统计、性能报告等功能

interface PerformanceMetric {
  label: string
  duration: number
  timestamp: number
  metadata?: Record<string, any>
}

interface PerformanceStats {
  count: number
  totalTime: number
  averageTime: number
  minTime: number
  maxTime: number
  recentTimes: number[]
}

export class PerformanceMonitor {
  private static metrics = new Map<string, PerformanceMetric[]>()
  private static maxMetricsPerLabel = 100 // 每个标签最多保留100条记录

  // 开始性能计时
  static startTiming(label: string, metadata?: Record<string, any>): () => void {
    const start = performance.now()

    return () => {
      const duration = performance.now() - start
      this.recordMetric(label, duration, metadata)
    }
  }

  // 记录性能指标（纯本地内存记录）
  static recordMetric(label: string, duration: number, metadata?: Record<string, any>) {
    if (!this.metrics.has(label)) {
      this.metrics.set(label, [])
    }

    const metrics = this.metrics.get(label)!
    const metric: PerformanceMetric = {
      label,
      duration,
      timestamp: Date.now(),
      metadata
    }

    metrics.push(metric)

    // 限制记录数量
    if (metrics.length > this.maxMetricsPerLabel) {
      metrics.splice(0, metrics.length - this.maxMetricsPerLabel)
    }

    // 在开发模式下输出慢操作警告
    if (import.meta.env.DEV && duration > 1000) {
      console.warn(`Slow operation detected: ${label} took ${duration.toFixed(2)}ms`, metadata)
    }
  }

  // 获取性能统计
  static getStats(label: string): PerformanceStats | null {
    const metrics = this.metrics.get(label)
    if (!metrics || metrics.length === 0) {
      return null
    }

    const durations = metrics.map(m => m.duration)
    const totalTime = durations.reduce((sum, d) => sum + d, 0)
    const recentTimes = durations.slice(-10) // 最近10次

    return {
      count: metrics.length,
      totalTime,
      averageTime: totalTime / metrics.length,
      minTime: Math.min(...durations),
      maxTime: Math.max(...durations),
      recentTimes
    }
  }

  // 获取所有标签的统计
  static getAllStats(): Record<string, PerformanceStats> {
    const result: Record<string, PerformanceStats> = {}

    for (const [label] of this.metrics) {
      const stats = this.getStats(label)
      if (stats) {
        result[label] = stats
      }
    }

    return result
  }

  // 获取平均时间
  static getAverageTime(label: string): number {
    const stats = this.getStats(label)
    return stats ? stats.averageTime : 0
  }

  // 获取最近的性能趋势
  static getRecentTrend(label: string, count: number = 10): number[] {
    const metrics = this.metrics.get(label)
    if (!metrics) return []

    return metrics
      .slice(-count)
      .map(m => m.duration)
  }

  // 检查性能是否有回归
  static checkPerformanceRegression(label: string, threshold: number = 1.5): boolean {
    const trend = this.getRecentTrend(label, 20)
    if (trend.length < 10) return false

    const recent = trend.slice(-5)
    const baseline = trend.slice(0, -5)

    if (baseline.length === 0) return false

    const recentAvg = recent.reduce((a, b) => a + b, 0) / recent.length
    const baselineAvg = baseline.reduce((a, b) => a + b, 0) / baseline.length

    return recentAvg > baselineAvg * threshold
  }

  // 清理过期指标
  static cleanup(maxAge: number = 60 * 60 * 1000) { // 默认1小时
    const cutoff = Date.now() - maxAge

    for (const [label, metrics] of this.metrics.entries()) {
      const filtered = metrics.filter(m => m.timestamp > cutoff)
      if (filtered.length === 0) {
        this.metrics.delete(label)
      } else {
        this.metrics.set(label, filtered)
      }
    }
  }

  // 重置所有指标
  static reset() {
    this.metrics.clear()
  }

  // 生成性能报告摘要
  static generateReport(): string {
    const allStats = this.getAllStats()
    const labels = Object.keys(allStats)

    if (labels.length === 0) {
      return 'No performance metrics recorded'
    }

    let report = '=== Performance Report ===\n\n'

    // 按总时间降序排序
    labels.sort((a, b) => allStats[b]!.totalTime - allStats[a]!.totalTime)

    for (const label of labels) {
      const stats = allStats[label]!
      report += `[${label}]\n`
      report += `  Count: ${stats.count}\n`
      report += `  Total: ${stats.totalTime.toFixed(2)}ms\n`
      report += `  Average: ${stats.averageTime.toFixed(2)}ms\n`
      report += `  Min: ${stats.minTime.toFixed(2)}ms\n`
      report += `  Max: ${stats.maxTime.toFixed(2)}ms\n`

      // 显示最近趋势
      const trend = this.getRecentTrend(label, 5)
      if (trend.length > 1) {
        const trendStr = trend.map(t => t.toFixed(1)).join(' -> ')
        report += `  Recent Trend: ${trendStr}\n`
      }

      // 检查是否有回归
      if (this.checkPerformanceRegression(label)) {
        report += `  WARNING: Performance regression detected!\n`
      }

      report += '\n'
    }

    return report
  }
}

// 性能装饰器：用于测量类方法的执行时间
export function measurePerformance(label?: string) {
  return function (
    _target: any,
    propertyKey: string,
    descriptor: PropertyDescriptor
  ) {
    const originalMethod = descriptor.value
    const metricLabel = label || propertyKey

    descriptor.value = function (...args: any[]) {
      const endTiming = PerformanceMonitor.startTiming(metricLabel)
      try {
        const result = originalMethod.apply(this, args)

        // 处理异步方法
        if (result instanceof Promise) {
          return result.finally(() => {
            endTiming()
          })
        }

        endTiming()
        return result
      } catch (error) {
        endTiming()
        throw error
      }
    }

    return descriptor
  }
}

// 便捷的性能计时函数
export function timeOperation<T>(label: string, operation: () => T, metadata?: Record<string, any>): T {
  const endTiming = PerformanceMonitor.startTiming(label, metadata)
  try {
    const result = operation()

    if (result instanceof Promise) {
      return result.finally(() => {
        endTiming()
      }) as unknown as T
    }

    endTiming()
    return result
  } catch (error) {
    endTiming()
    throw error
  }
}

// Vue 组合式 API 支持
export function usePerformanceMonitor() {
  const startTiming = (label: string, metadata?: Record<string, any>) => {
    return PerformanceMonitor.startTiming(label, metadata)
  }

  const recordMetric = (label: string, duration: number, metadata?: Record<string, any>) => {
    PerformanceMonitor.recordMetric(label, duration, metadata)
  }

  const getStats = (label: string) => {
    return PerformanceMonitor.getStats(label)
  }

  const getAllStats = () => {
    return PerformanceMonitor.getAllStats()
  }

  return {
    startTiming,
    recordMetric,
    getStats,
    getAllStats
  }
}
