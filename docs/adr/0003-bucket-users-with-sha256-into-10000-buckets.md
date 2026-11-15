# Bucket users with SHA-256 into 10,000 buckets

A percentage rollout puts each user into a bucket from 0 to 9,999: the first 8 bytes of
`sha256("<flag key>.<user key>")`, read as a big-endian unsigned integer, modulo 10,000. A user gets `true` when their
bucket is below `true_percent * 100`. The same input always gives the same bucket, so a user's answer is stable; adding
the flag key means each flag picks a different slice of users; and raising the percentage only moves the threshold up,
so users who already had the feature keep it. 10,000 buckets leave room for 0.01% steps later even though the API
only takes whole percentages today. This scheme is hard to change: any change to the hash input, the hash function or
the bucket count reshuffles every user of every running rollout, and the SDK must compute exactly the same buckets as
the server, which is why `TestBucketKnownValues` pins three known results.
