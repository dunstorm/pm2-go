import time
for i in range(500):
    print(str(i))
    if i > 300:
        raise Exception("an_error")
    time.sleep(1)