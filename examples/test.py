import time
for i in range(5):
    print(str(i))
    if i > 2:
        raise Exception("an_error")
    time.sleep(1)