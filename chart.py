import pandas as pd
import matplotlib.pyplot as plt
import os
# 读取 export.json 文件
with open("log/export.json", "r") as f:
    df = pd.read_json(f, orient='records') 

# 创建pic目录（如果不存在）
if not os.path.exists("export"):
    os.mkdir("export")

# 绘制图表并保存
plt.plot(df["Second"], df["RateBytes"]/1024/1024)
plt.xlabel("Second")
plt.ylabel("Rate MB/s")
plt.title("Rate By Second")
plt.savefig("export/example.png")
plt.show()