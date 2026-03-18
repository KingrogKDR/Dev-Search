package queues

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	ReadyKey      = "%s:ready:%s"
	ProcessingKey = "processing:%s"
	ResultsKey    = "results:%s"
	FailedKey     = "%s:failed"
	RetryKey      = "%s:retry"

	ProcessingTimeout = 5 * time.Minute
)

type Queue struct {
	Redis     *redis.Client
	ctx       context.Context
	namespace string
}

func NewQueue(redisClient *redis.Client, namespace string) *Queue {
	return &Queue{
		Redis:     redisClient,
		ctx:       context.Background(),
		namespace: namespace,
	}
}

func (q *Queue) Enqueue(job *Job) error {
	var effectiveScore int

	if job.LastEnqueuedAt.IsZero() {
		effectiveScore = job.BaseScore
	} else {
		waited := time.Since(job.LastEnqueuedAt)
		effectiveScore = ApplyAging(job.BaseScore, waited)
	}

	job.Priority = ScoreToPriority(effectiveScore)
	job.LastEnqueuedAt = time.Now()

	jobData, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("Failed to marshal job: %w", err)
	}

	queueKey := fmt.Sprintf(ReadyKey, q.namespace, string(job.Priority))

	err = q.Redis.RPush(q.ctx, queueKey, jobData).Err()
	if err != nil {
		return fmt.Errorf("Failed to enqueue task: %w", err)
	}

	log.Printf("Job %s enqueued to '%s' queue", job.ID, job.Priority)

	return nil
}

func (q *Queue) RequeueWithDelay(job *Job, delay time.Duration) error {

	data, err := json.Marshal(job)
	if err != nil {
		return err
	}

	retryTime := time.Now().Add(delay).Unix()
	retryKey := fmt.Sprintf(RetryKey, q.namespace)
	return q.Redis.ZAdd(q.ctx, retryKey, redis.Z{
		Score:  float64(retryTime),
		Member: string(data),
	}).Err()
}

func (q *Queue) Dequeue(queues []string, workerID string, timeout time.Duration) (*Job, error) {
	queueKeys := make([]string, len(queues))
	for i, queue := range queues {
		queueKeys[i] = fmt.Sprintf(ReadyKey, q.namespace, queue)
	}

	result, err := q.Redis.BLPop(q.ctx, timeout, queueKeys...).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil // no available jobs
		}
		return nil, fmt.Errorf("Failed to dequeue task: %w", err)
	}

	var job Job

	if err := json.Unmarshal([]byte(result[1]), &job); err != nil {
		return nil, fmt.Errorf("Failed to unmarshal task: %w", err)
	}

	processingKey := fmt.Sprintf(ProcessingKey, workerID)

	job.Status = JOB_INFLIGHT
	job.VisibilityStart = time.Now()
	updatedJobData, _ := json.Marshal(job)
	pipe := q.Redis.Pipeline()
	pipe.Set(q.ctx, "job:"+job.ID, updatedJobData, ProcessingTimeout)
	pipe.LPush(q.ctx, processingKey, job.ID)
	pipe.Expire(q.ctx, processingKey, ProcessingTimeout)

	_, err = pipe.Exec(q.ctx)

	if err != nil {
		log.Printf("Warning: failed to move task to processing: %v", err)
	}

	log.Printf("Job for '%s' moved to processing state!", job.URL)

	return &job, nil
}

func (q *Queue) CompleteJob(job *Job, result *Result, workerID string) error {
	processingKey := fmt.Sprintf(ProcessingKey, workerID)
	resultKey := fmt.Sprintf(ResultsKey, job.ID)

	resultData, _ := json.Marshal(result)

	pipe := q.Redis.Pipeline()

	pipe.LRem(q.ctx, processingKey, 1, job.ID)
	pipe.Del(q.ctx, "job:"+job.ID)

	pipe.Set(q.ctx, resultKey, resultData, 24*time.Hour)

	if !result.Success {
		if job.RetryCount >= MAX_RETRIES {
			job.Status = JOB_DEAD
			jobData, _ := json.Marshal(job)
			failedKey := fmt.Sprintf(FailedKey, q.namespace)
			pipe.LPush(q.ctx, failedKey, jobData)
		} else {
			job.RetryCount++

			job.BaseScore -= max(10, job.RetryCount*10)

			if job.BaseScore < 0 {
				job.BaseScore = 10
			}

			job.Priority = ScoreToPriority(job.BaseScore)

			delay := time.Second * time.Duration(1<<job.RetryCount)
			retryTime := time.Now().Add(delay).Unix()

			jobData, _ := json.Marshal(job)
			retryKey := fmt.Sprintf(RetryKey, q.namespace)

			pipe.ZAdd(q.ctx, retryKey, redis.Z{
				Score:  float64(retryTime),
				Member: string(jobData),
			})

		}
	} else {
		job.Status = JOB_DONE
		log.Printf("Job %s for %s successfully completed!", job.ID, job.URL)
	}

	_, err := pipe.Exec(q.ctx)
	return err
}

func (q *Queue) ProcessRetryJobs() error {
	now := float64(time.Now().Unix())
	retryKey := fmt.Sprintf(RetryKey, q.namespace)
	results, err := q.Redis.ZRangeByScoreWithScores(q.ctx, retryKey, &redis.ZRangeBy{
		Min: "0",
		Max: fmt.Sprintf("%f", now),
	}).Result()

	if err != nil {
		return err
	}

	if len(results) == 0 {
		return nil
	}

	pipe := q.Redis.Pipeline()

	for _, result := range results {
		jobData := result.Member.(string)

		var job Job
		if err = json.Unmarshal([]byte(jobData), &job); err != nil {
			continue
		}
		waited := time.Since(job.LastEnqueuedAt)
		job.BaseScore = ApplyAging(job.BaseScore, waited)
		job.Priority = ScoreToPriority(job.BaseScore)
		job.LastEnqueuedAt = time.Now()
		job.Status = JOB_READY

		newJobData, _ := json.Marshal(job)

		queueKey := fmt.Sprintf(ReadyKey, q.namespace, string(job.Priority))

		pipe.RPush(q.ctx, queueKey, newJobData)
		pipe.ZRem(q.ctx, retryKey, jobData)
	}

	_, err = pipe.Exec(q.ctx)

	if err != nil {
		return err
	}

	log.Printf("Moved %d retry jobs for %s to queues", len(results), q.namespace)
	return nil
}
func (q *Queue) ReapStaleProcessingJobs(workerID string) error {
	processingKey := fmt.Sprintf(ProcessingKey, workerID)

	jobIDs, err := q.Redis.LRange(q.ctx, processingKey, 0, -1).Result()
	if err != nil {
		return err
	}

	now := time.Now()

	for _, jobID := range jobIDs {
		data, err := q.Redis.Get(q.ctx, "job:"+jobID).Bytes()
		if err != nil {
			continue
		}
		var job Job

		if err := json.Unmarshal(data, &job); err != nil {
			continue
		}

		if now.Sub(job.VisibilityStart) < ProcessingTimeout {
			continue
		}

		log.Printf("Reaping stuck job: %s", job.ID)

		job.Status = JOB_READY
		job.LastEnqueuedAt = time.Now()

		newJobData, _ := json.Marshal(job)

		queueKey := fmt.Sprintf(ReadyKey, q.namespace, string(job.Priority))

		pipe := q.Redis.Pipeline()

		pipe.LRem(q.ctx, processingKey, 1, jobID)
		pipe.Del(q.ctx, "job:"+jobID)
		pipe.RPush(q.ctx, queueKey, newJobData)

		_, err = pipe.Exec(q.ctx)
		if err != nil {
			return err
		}
	}

	return nil
}
